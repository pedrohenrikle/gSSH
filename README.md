# gSSH

gSSH is a remote connection tool built in Go, leveraging gRPC to streamline secure server-client communication. It features TLS/SSL encryption for secure data transmission, ensuring confidentiality and integrity of interactions.

## Features

- **Secure Communication:** TLS/SSL encryption to ensure data confidentiality;
- **Remote Access:** Execute commands remotely across servers;
- **Built with gRPC:** Leveraging gRPC for efficient and scalable remote communication;

## Getting Started

### Prerequisites
- Go 1.18+
- OpenSSL (for generating certificates)

### Installation
1. Clone the repository:
    ```sh
    git clone https://github.com/pedrohenrikle/gSSH.git
    cd gSSH
    ```

2. Generate TLS/SSL certificates (for development):
    ```sh
    mkdir -p cert
    openssl req -x509 -newkey rsa:4096 -keyout cert/server.key -out cert/server.crt -days 365 -nodes -subj "/CN=localhost"
    ```

3. Install dependencies:
    ```sh
    go mod download
    ```

4. Setup environment variables:
    ```sh
    touch .env
    ```
    And follow the example on [`.env example`](https://github.com/pedrohenrikle/gSSH/blob/main/.env.example).

### Running the Server
```sh
go run cmd/server/server.go --port=<port>
```

or build as:

```sh
mkdir -p out
go build -o out/server cmd/server/server.go
./out/server
```

### Running the Client
```sh
go run cmd/client/client.go --id=<session_id> --port=<port>
```

or build as:

```sh
mkdir -p out
go build -o out/client cmd/client/client.go
./out/client
```

### Command-Line Flags and Environment Variables

- #### Client Flags:

    - `--id`: (Optional) Session ID to run to an existing session.

    - `--port`: (Optional) Port to run the TCP connection with the server.

- #### Server Flags:

    - `--port`: (Optional) Determines the port to run the TCP conection.


## Project Structure
- `cert/`: Contains TLS/SSL certificates;
- `cmd/client/`: Client code to connect and interact with the server;
- `cmd/server/`: Server code to handle client requests;
- `pkg`: Contains packages that encapsulate different functionalities. 
- `proto/`: Protocol buffer definitions for gRPC;
- `pb/`: Protocol buffer auto-generated files that define data structures and service interfaces for gRPC;
- `out/`: Directory to compiled/build binaries; 

## License
This project is licensed under the MIT License.
