package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"gSSH/pb"
	"io"
	"log"
	"net/http"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var url = "http://localhost:8080/cert"

func fetchCertificate(url string) ([]byte, error) { // perform a GET request to fetch the certificate
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cert: %v", err)
	}
	defer resp.Body.Close() // read the response body
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

	// Create a certificate pool
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(cert); !ok {
		log.Fatalf("failed to append cert to pool")
	}

	// Create TLS credentials
	creds := credentials.NewTLS(&tls.Config{RootCAs: certPool})

	dial, err := grpc.NewClient(
		"localhost:50052",
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		panic(err)
	}
	defer dial.Close()

	client := pb.NewCommandServiceClient(dial)

	stream, err := client.ExecuteCommand(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println("Client connected with TLS/SSL!")

	// anonymous function to recive the responses
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
		err := stream.Send(&pb.CommandRequest{Command: command})
		if err != nil {
			log.Fatalf("error sending command: %v", err)
		}

	}

	<-done

}
