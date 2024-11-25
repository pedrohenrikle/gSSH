package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	env "gSSH/cmd"
	"gSSH/pb"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var environment = env.NewEnv()

func init() {
	// Set default values
	viper.SetDefault("port", environment.ServerPort)
	viper.SetDefault("id", "")

	// Command-line flags
	pflag.Int("port", environment.ServerPort, "Port to run the TCP connection")
	pflag.String("id", "", "Session ID")

	pflag.Parse()

	// Bind the flags to viper
	viper.BindPFlag("port", pflag.Lookup("port"))
	viper.BindPFlag("id", pflag.Lookup("id"))

	// Environment variables
	viper.BindEnv("id", "SESSION_ID")
	viper.BindEnv("port", "SERVER_PORT")
}

func fetchCertificate(url string) ([]byte, error) { // Perform a GET request to fetch the certificate
	resp, err := http.Get("http://" + url + "/cert")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cert: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch cert: server returned %v", resp.Status)
	}

	cert, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read cert body: %v", err)
	}
	return cert, nil
}

func main() {
	port := viper.GetInt("port")
	sessionID := viper.GetString("id")

	address := fmt.Sprintf(":%d", port)

	fmt.Printf("Starting client on address: %s...\n", address)

	certPortStr := strconv.Itoa(environment.ServerCertPort)
	certAddress := environment.ServerAddress + ":" + certPortStr

	cert, err := fetchCertificate(certAddress)
	if err != nil {
		log.Fatalf("failed to fetch cert: %v", err)
	}

	// Create a certificate pool
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(cert); !ok {
		log.Fatalf("failed to append cert to pool: invalid PEM format or empty certificate")
	}

	creds := credentials.NewTLS(&tls.Config{RootCAs: certPool})

	TCPaddress := fmt.Sprintf("%s:%d", environment.ServerAddress, port)

	socket, err := grpc.NewClient(
		TCPaddress,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		panic(err)
	}
	defer socket.Close()

	client := pb.NewTerminalServiceClient(socket)

	var sessionRes *pb.SessionResponse
	fmt.Printf("sessionID: %s\n", sessionID)
	sessionRes, err = client.RequestSession(context.Background(), &pb.SessionRequest{Id: &sessionID})

	if err != nil {
		log.Fatalf("failed to request session: %v", err)
	}

	if sessionRes.SessionStatus != pb.SessionStatus_AVAILABLE {
		log.Fatalf("session not available: %v", sessionRes.SessionStatus)
	}

	// Update sessionID with the ID received from the server if it was generated there
	if sessionID == "" {
		sessionID = sessionRes.Id
	}

	stream, err := client.ExecuteCommand(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println("Client connected with TLS!")

	// Anonymous function to receive the responses
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
			SessionId: sessionID,
		})
		if err != nil {
			log.Fatalf("error sending command: %v", err)
		}
	}

	<-done
}
