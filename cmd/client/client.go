package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"gSSH/pb"
	"io"
	"log"
	"net/http"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	url       = "http://localhost:8080/cert"
	sessionID *string
)

func init() {
	id := flag.String("id", "", "Session ID")
	flag.Parse()
	if *id != "" {
		sessionID = id
	}
}

func fetchCertificate(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cert: %v", err)
	}
	defer resp.Body.Close()
	cert, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read cert body: %v", err)
	}
	return cert, nil
}

func main() {
	fmt.Println("Starting client...")

	cert, err := fetchCertificate(url)
	if err != nil {
		log.Fatalf("failed to fetch cert: %v", err)
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(cert); !ok {
		log.Fatalf("failed to append cert to pool")
	}

	creds := credentials.NewTLS(&tls.Config{RootCAs: certPool})

	conn, err := grpc.NewClient("localhost:50052", grpc.WithTransportCredentials(creds))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	client := pb.NewTerminalServiceClient(conn)

	var sessionRes *pb.SessionResponse
	sessionRes, err = client.RequestSession(context.Background(), &pb.SessionRequest{Id: sessionID})

	fmt.Printf(sessionRes.SessionStatus.String())

	if err != nil {
		log.Fatalf("failed to request session: %v", err)
	}

	if sessionRes.SessionStatus != pb.SessionStatus_AVAILABLE {
		log.Fatalf("session not available: %v", sessionRes.SessionStatus)
	}

	// Update sessionID with the ID received from the server if it was generated there
	if sessionID == nil {
		sessionID = &sessionRes.Id
	}

	stream, err := client.ExecuteCommand(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Println("Client connected with TLS/SSL!")

	done := make(chan bool)
	go func() {
		for {
			response, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}

			fmt.Printf(response.Output)
		}
		close(done)
	}()

	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		command := scanner.Text()
		err := stream.Send(&pb.CommandRequest{
			Command:   command,
			SessionId: *sessionID,
		})
		if err != nil {
			log.Fatalf("error sending command: %v", err)
		}
	}

	<-done
}
