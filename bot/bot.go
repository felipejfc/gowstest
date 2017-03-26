package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var quitChan chan bool
var serverAddr string
var numBots int
var intervalBetweenMessages float32
var wg sync.WaitGroup
var msgTemplate string

var sentMessages int64
var receivedMessages int64

func linesFromReader(r io.Reader) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func fileToLines(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return linesFromReader(f)
}

func readParams() {
	serverAddr = os.Getenv("SERVER_ADDR")
	if serverAddr == "" {
		serverAddr = "localhost:8080"
	}
	nBots := os.Getenv("NUM_BOTS")
	if nBots != "" {
		n, err := strconv.ParseInt(nBots, 10, 64)
		if err != nil {
			panic(err.Error())
		}
		numBots = int(n)
	} else {
		numBots = 200
	}

	intervalBetweenMessages = 1000.0 / 15.0
}

func listenPlayerMessages(playerID string, c *websocket.Conn) {
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			return
		}
		//fmt.Printf("Received message: %s\n", string(message))
		if !strings.HasPrefix(string(message), playerID) {
			log.Printf("Message arrived for wrong player. PlayerID: %s, Message: %s\n", playerID, string(message))
			return
		}
		atomic.AddInt64(&receivedMessages, 1)
	}
}

func connectPlayer(playerID string) *websocket.Conn {
	u := url.URL{Scheme: "ws", Host: serverAddr, Path: fmt.Sprintf("/%s", playerID)}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}

	go listenPlayerMessages(playerID, c)

	return c
}

func getRandomEnemy(playerID string, ids []string) string {
	if len(ids) < 2 {
		return ""
	}

	for {
		id := ids[rand.Intn(len(ids))]
		if id != playerID {
			return id
		}
	}
}

func sendPlayerMessages(playerID string, ids []string, c *websocket.Conn) {
	wg.Add(1)
	defer func() {
		err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Println("write close:", err)
			return
		}
		c.Close()
		wg.Done()
	}()

	tickChan := time.NewTicker(time.Millisecond * time.Duration(intervalBetweenMessages)).C

	for {
		select {
		case <-quitChan:
			return
		case <-tickChan:
			err := c.WriteMessage(
				websocket.TextMessage,
				[]byte(fmt.Sprintf(msgTemplate, getRandomEnemy(playerID, ids))),
			)
			if err != nil {
				log.Println("write:", err)
				return
			}
			atomic.AddInt64(&sentMessages, 1)
		}
	}
}

func printDetails(duration float64) {
	fmt.Printf("Total of sent messages: %d\n", sentMessages)
	fmt.Printf("Total of received messages: %d\n", receivedMessages)
	fmt.Printf("Percentage of received messages: %.2f%%\n", (float64(receivedMessages) / float64(sentMessages) * 100))
	fmt.Printf("Sent messages rate: %.2f/sec\n", float64(sentMessages)/duration)
	fmt.Printf("Received messages rate: %.2f/sec\n", float64(receivedMessages)/duration)
}

func main() {
	start := time.Now()
	quitChan = make(chan bool)
	msgTemplate = "%s,4.23124,3.1415,8.5828348,270.42198842,184.24928148"
	ids, err := fileToLines("./ids")
	if err != nil {
		panic(err)
	}
	readParams()
	log.Printf("Starting %d bots...\n", numBots)
	for i := 0; i < numBots; i++ {
		playerID := ids[i]
		c := connectPlayer(playerID)
		go sendPlayerMessages(playerID, ids, c)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		log.Println("Ctrl+C detected. Quitting...")
		duration := time.Now().Sub(start)
		printDetails(duration.Seconds())
		close(quitChan)
		os.Exit(0)
	}()

	wg.Wait()
}
