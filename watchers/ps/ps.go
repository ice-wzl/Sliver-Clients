package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/bishopfox/sliver/client/assets"
	"github.com/bishopfox/sliver/client/console"
	"github.com/bishopfox/sliver/client/transport"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/rpcpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
	"github.com/jedib0t/go-pretty/v6/table"
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
		TopSeparator:     "",
		BottomSeparator:  "",
		Left:             "",
		Right:            "",
		TopLeft:          "",
		TopRight:         "",
		BottomLeft:       "",
		BottomRight:      "",
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

func clearScreen() {
	cmd := exec.Command("clear") // 'clear' command for Linux/Mac
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// Function handles when multiple sessions are active on the sliver server
//
// :param: sessions []*clientpb.Session -> an array of active sessions connected to the sliver server
// :return: *clientpb.Session -> the client session the operator wishes to connect to 
func handleMultipleSessions(sessions []*clientpb.Session) *clientpb.Session {
	fmt.Println("[+] Multiple Sessions Detected [+]")
	fmt.Println("[+] Which session should this module be run against:")

	tw := table.NewWriter()
	//tw.SetStyle(settings.GetTableWithBordersStyle(con))
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
		//fmt.Printf("%d. %s   %s   %s\n",index, i.ID, i.Hostname, i.RemoteAddress)
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

func main() {
	var configPath string
	var sleepTime int
	flag.StringVar(&configPath, "config", "", "path to sliver client config file")
	flag.IntVar(&sleepTime, "sleep", 60, "the time to sleep in between process list polling")
	flag.Parse()

	if configPath == "" {
		fmt.Println("[!] Specify a client config to load")
		os.Exit(1)
	}

	sessions, rpc, ln := makeConnection(&configPath)
	defer ln.Close()

	var targetSession *clientpb.Session
	//fmt.Println(sessions)
	//fmt.Println(len(sessions.Sessions))
	if len(sessions.Sessions) > 1 {
		// need to handle multiple clients connected
		targetSession = handleMultipleSessions(sessions.Sessions)
		
	} else {
		targetSession = sessions.Sessions[0]
	}

	//targetSession := sessions.Sessions[0]

	//fileTag := fmt.Sprintf(targetSession.RemoteAddress)
	for {
		processList(targetSession, rpc)
		time.Sleep(time.Duration(sleepTime) * time.Second)
		clearScreen()
	}
}
