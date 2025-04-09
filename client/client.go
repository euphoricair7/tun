package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func main() {
	serverIP := "SHAILJA-IP-ADDRESS"
	serverPort := "CHECK-PORT-NUMBER"

	serverAddr := serverIP + ":" + serverPort

	connection,err := net.Dial("tcp", serverAddr)
	if err != nil {
		fmt.Println("Error in connecting to the server -- : ",err)
		return
	}
	defer connection.Close()

	fmt.Println("Connected to the server successfully at :", serverAddr)

	localPort := "SOME_LOCAL_PORT_LIKE_3000"
	message := "REQUEST_PORT_FOR:" + localPort + "\n"
	_,err =connection.Write([]byte(message))
	if err != nil {
		fmt.Println("Error in sending a request to the server to request for a public port -- :",err)
		return
	}
	
	reader := bufio.NewReader(connection)
	//reader is a buffer that reads from the connection.
	//NewReader function creates a new reader that reads from the connection.
	assignedPort,err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading the assigned port from the server -- :",err)
		return
	}

	fmt.Println("Assigned port is : ", assignedPort)

    startTunneling(assignedPort, localPort)
	/*reader := bufio.NewReader(os.Stdin)
	for{
		fmt.Print("Enter the message or type 'exit' to quit :")
		text,_ := reader.ReadString('\n')
		//ReadString function reads until the delimiter '\n' and returns the string and an error if any.

		if text == "exit\n" {
			fmt.Println("Closing connection .")
			break
		}

		_,err := connection.Write([]byte(text))
		//Write function returns the number of bytes written and an error if any.
		if err != nil {
			fmt.Println("Error sending message to the server -- :",err)
			break
		}

	}
	*/
}

func startTunneling(publicPort , localPort string) {
	fmt.Println("Starting tunnel: Public port", pubicPort, "-> Local port ", localPort)

	localListener,err := net.Listener("tcp", "localhost:"+ localPort)
	if err !=nil {
		fmt.Println("Error in setting up a local listener --:",err)
		return 
	}

	defer localListener.Close()

	for {
		clientConnection,err := localListener.Accept()
		if err != nil {
			fmt.Println("Error accepting local connection -- :",err)
			continue
		}
		go handleClient(clientConnection)

	}
	
}

func handleClient(clientConnection net.Conn) {
	defer clientConnection.Close()

    fmt.Println("New conection successfully established.")

	reader := bufio.NewReader(clientConnection)
	for {
		data,err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("error in reading data from the client. Connection closed -- :",err)
			break
		}
		fmt.Println("Data read from the user :" ,data)

	}
}