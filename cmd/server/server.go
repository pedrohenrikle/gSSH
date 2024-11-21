package main

import (
	"fmt"
	env "gSSH/cmd"
	"gSSH/pb"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/creack/pty"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Server struct {
	pb.UnimplementedCommandServiceServer
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

func (s *Server) ExecuteCommand(stream pb.CommandService_ExecuteCommandServer) error {
	// start a bash session
	bashSession := exec.Command("bash")
	ptmx, err := pty.Start(bashSession)
	if err != nil {
		return err
	}
	defer func() { _ = ptmx.Close() }() // close pty when finish

	// disable 'echo' mode to not duplicate visually the commands on output
	termState, err := term.MakeRaw(int(ptmx.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(ptmx.Fd()), termState)

	// configure sinal to resize terminal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("Erro ao redimensionar o PTY: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH                        // initial resizing
	defer func() { signal.Stop(ch); close(ch) }() // cleaning sinals on finish

	// goroutine to send the session output to client
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if err != nil && err != io.EOF {
				log.Println("Erro ao ler do PTY:", err)
				return
			}
			if n == 0 {
				continue
			}

			// send output to client
			if err := stream.Send(&pb.CommandResponse{Output: string(buf[:n])}); err != nil {
				log.Println("Erro ao enviar resposta:", err)
				return
			}
		}
	}()

	// goroutine to monitorate the bash process
	go func() {
		if err := bashSession.Wait(); err != nil {
			log.Println("Sessão bash encerrada:", err)
		}
		// Envia mensagem especial de término da sessão ao cliente
		stream.Send(&pb.CommandResponse{Output: "Sessão encerrada"})
	}()

	// loop to recive client commands and copy to pty
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// write recived command on pty
		if _, err := ptmx.Write([]byte(req.Command + "\n")); err != nil {
			return err
		}
	}
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

	// create the TLS credentials
	creds, err := credentials.NewServerTLSFromFile(crt, key)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Listening on %s with TLS...\n", address)

	// combine ServerAddress and ServerCertPort to create certAddress
	certPortStr := strconv.Itoa(environment.ServerCertPort)
	certAddress := environment.ServerAddress + ":" + certPortStr

	// serve the certificate via HTTP
	http.HandleFunc("/cert", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "./cert/server.crt") })
	go http.ListenAndServe(certAddress, nil)

	s := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterCommandServiceServer(s, &Server{})

	fmt.Println("Serving gRPC...")

	if err := s.Serve(socket); err != nil {
		panic(err)
	}
}
