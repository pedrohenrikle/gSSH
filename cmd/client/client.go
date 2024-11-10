package main

import (
	"bufio"
	"context"
	"fmt"
	"gSSH/pb"
	"io"
	"log"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var cert = "cert/server.crt"

func main() {
	fmt.Println("Starting client...")

	// Create the client TLS credentials
	creds, err := credentials.NewClientTLSFromFile(cert, "")
	if err != nil {
		panic(err)
	}

	dial, err := grpc.NewClient(
		":50052",
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
	fmt.Println("Client connected with TCL/SSL!")

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
