package main

import (
	"context"
	"fmt"
	env "gSSH/cmd"
	"gSSH/pb"
	"gSSH/pkg/session"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/google/uuid"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedTerminalServiceServer
	sessions   map[string]*session.BashSession
	sessionMux sync.Mutex
}

type BashSession struct {
	Id              string
	TerminalCommand *exec.Cmd
	Ptmx            *os.File
	InUse           bool
}

var (
	crt         = "cert/server.crt"
	key         = "cert/server.key"
	environment = env.NewEnv()
)

func init() {
	viper.SetDefault("port", environment.ServerPort)

	pflag.Int("port", environment.ServerPort, "Port to run the TCP connection")
	pflag.Parse()

	viper.BindPFlag("port", pflag.Lookup("port"))

	viper.BindEnv("port", "SERVER_PORT")
}

func generateSessionId() string {
	id := uuid.New()
	return id.String()
}

func (s *Server) RequestSession(ctx context.Context, req *pb.SessionRequest) (*pb.SessionResponse, error) {
	var sessionId string

	if req.GetId() != "" {
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
		}
	} else {
		newSession, err := session.New(sessionId)
		if err != nil {
			return &pb.SessionResponse{}, err
		}

		s.sessions[sessionId] = newSession
		fmt.Printf("Created new session %s and marked as in use.\n", sessionId)
	}

	return &pb.SessionResponse{
		Id:            sessionId,
		SessionStatus: pb.SessionStatus_AVAILABLE,
	}, nil
}

func (s *Server) ExecuteCommand(stream pb.TerminalService_ExecuteCommandServer) error {
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
	defer func() { _ = ptmx.Close() }()

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

func (s *Server) MakeSessionAvailable(ctx context.Context, req *pb.SessionRequest) (*pb.SessionResponse, error) {
	sessionId := req.Id

	s.sessionMux.Lock()
	defer s.sessionMux.Unlock()

	if session, ok := s.sessions[*sessionId]; ok {
		if session.Ptmx != nil {
			if err := session.Ptmx.Close(); err != nil {
				return nil, fmt.Errorf("failed to close PTY: %v", err)
			}

			newSession, _ := session.New(*sessionId)
			newSession.InUse = false
			s.sessions[*sessionId] = newSession
		}

		fmt.Printf("Session liberated for use: %s", *sessionId)
		fmt.Printf("%v", s.sessions[*sessionId])
		return &pb.SessionResponse{
			Id:            *sessionId,
			SessionStatus: pb.SessionStatus_AVAILABLE,
		}, nil
	}

	return &pb.SessionResponse{
		Id:            *sessionId,
		SessionStatus: pb.SessionStatus_TERMINATED,
	}, nil
}

func main() {
	port := viper.GetInt("port")

	address := fmt.Sprintf("%s:%d", environment.ServerAddress, port)
	fmt.Printf("Starting server on address: %s...\n", address)

	socket, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}
	defer socket.Close()

	creds, err := credentials.NewServerTLSFromFile(crt, key)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Listening on %s with TLS...\n", address)

	// Combine ServerAddress and ServerCertPort to create certAddress
	certPortStr := strconv.Itoa(environment.ServerCertPort)
	certAddress := environment.ServerAddress + ":" + certPortStr

	// Serve the certificate via HTTP
	http.HandleFunc("/cert", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "./cert/server.crt") })
	go http.ListenAndServe(certAddress, nil)

	server := &Server{
		sessions: make(map[string]*session.BashSession),
	}

	s := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterTerminalServiceServer(s, server)

	fmt.Println("Serving gRPC...")

	if err := s.Serve(socket); err != nil {
		panic(err)
	}
}
