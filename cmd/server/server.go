package main

import (
	"context"
	"fmt"
	"gSSH/pb"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedTerminalServiceServer
	sessions   map[string]*BashSession
	sessionMux sync.Mutex
}

type BashSession struct {
	Id              string
	TerminalCommand *exec.Cmd
	Ptmx            *os.File
	InUse           bool
}

var (
	crt = "cert/server.crt"
	key = "cert/server.key"
)

func generateSessionId() string {
	fmt.Println(time.Now().UnixNano())
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *Server) RequestSession(ctx context.Context, req *pb.SessionRequest) (*pb.SessionResponse, error) {
	var sessionId string

	if req.Id != nil {
		sessionId = req.GetId()
		fmt.Printf("Requested sessionId: %s\n", sessionId)
	} else {
		sessionId = generateSessionId()
		fmt.Printf("Generated new sessionId: %s\n", sessionId)
	}

	s.sessionMux.Lock()
	defer s.sessionMux.Unlock()

	if session, exists := s.sessions[sessionId]; exists {
		if session.InUse {
			fmt.Printf("Session %s is in use.\n", sessionId)
			return &pb.SessionResponse{
				Id:            sessionId,
				SessionStatus: pb.SessionStatus_IN_USE,
			}, nil
		} else {
			session.InUse = true // Mark session as in use
			fmt.Printf("Marked session %s as in use.\n", sessionId)
		}
	} else {
		// Initialize a bash session and a PTY session
		bashSession := exec.Command("bash")
		ptmx, err := pty.Start(bashSession)
		if err != nil {
			fmt.Printf("Failed to start bash session for %s: %v\n", sessionId, err)
			return nil, err
		}

		termState, err := term.MakeRaw(int(ptmx.Fd()))
		if err != nil {
			fmt.Printf("Failed to set raw mode for %s: %v\n", sessionId, err)
			return nil, err
		}
		defer term.Restore(int(ptmx.Fd()), termState)

		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)

		// This ensures that the PTY adjusts to terminal window size changes.
		go func() {
			for range ch {
				if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
					log.Fatalf("error trying to resize the PTY: %v", err)
				}
			}
		}()
		ch <- syscall.SIGWINCH
		defer func() { signal.Stop(ch); close(ch) }()

		s.sessions[sessionId] = &BashSession{
			Id:              sessionId,
			TerminalCommand: bashSession,
			Ptmx:            ptmx,
			InUse:           true, // Mark session as in use
		}
		fmt.Printf("Created new session %s and marked as in use.\n", sessionId)
	}

	return &pb.SessionResponse{
		Id:            sessionId,
		SessionStatus: pb.SessionStatus_AVAILABLE,
	}, nil
}

func (s *Server) ExecuteCommand(stream pb.TerminalService_ExecuteCommandServer) error {
	var bashSession *BashSession
	var err error

	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to receive initial request: %v", err)
	}

	sessionId := req.SessionId
	fmt.Printf("Executing command for sessionId: %s\n", sessionId)

	s.sessionMux.Lock()
	bashSession, ok := s.sessions[sessionId]
	if !ok {
		s.sessionMux.Unlock()
		return status.Errorf(codes.NotFound, "session not found: %s", sessionId)
	}
	bashSession.InUse = true // Mark session as in use
	s.sessionMux.Unlock()
	fmt.Printf("Marked session %s as in use during ExecuteCommand.\n", sessionId)

	ptmx := bashSession.Ptmx

	// Goroutine to send the session output to client
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				log.Fatalf("error trying to read from PTY: %v", err)
				return
			}
			if n == 0 {
				continue
			}

			// Send output to client
			if err := stream.Send(&pb.CommandResponse{Output: string(buf[:n])}); err != nil {
				log.Fatalf("error trying to send response: %v", err)
				return
			}
		}
	}()

	// Loop to receive client commands and copy to PTY
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			s.sessionMux.Lock()
			bashSession.InUse = false // Mark session as not in use
			s.sessionMux.Unlock()
			fmt.Printf("Marked session %s as not in use after EOF.\n", sessionId)
			return nil
		}
		if err != nil {
			return err
		}

		// Write received command on PTY
		if _, err := ptmx.Write([]byte(req.Command + "\n")); err != nil {
			return err
		}
	}
}

func main() {
	fmt.Println("Starting server...")
	socket, err := net.Listen("tcp", ":50052")
	if err != nil {
		panic(err)
	}
	defer socket.Close()

	creds, err := credentials.NewServerTLSFromFile(crt, key)
	if err != nil {
		panic(err)
	}

	fmt.Println("Listening on :50052 with TLS/SSL...")

	// HTTP server to send certificate to client
	http.HandleFunc("/cert", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "./cert/server.crt") })
	go http.ListenAndServe("localhost:8080", nil)

	server := &Server{
		sessions: make(map[string]*BashSession),
	}

	s := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterTerminalServiceServer(s, server)

	fmt.Println("Serving gRPC...")

	if err := s.Serve(socket); err != nil {
		panic(err)
	}
}
