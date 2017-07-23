package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/nlopes/slack"
)

var messagesChannel chan string
var postMessagesChannel chan string
var control chan bool

const (
	channelBufferSize   = 23
	messageChannelDepth = 42
)

type config struct {
	Secret   string `json:"secret"`
	Channel  string `json:"channel"`
	Topic    string `json:"topic"`
	Debug    bool   `json:"debug"`
	Broker   string `json:"broker"`
	Port     string `json:"port"`
	ClientID string `json:"clientID"`
}

var handleSubscribedInboundMQTT mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	postMessagesChannel <- string(msg.Payload())
	fmt.Printf("MID: %d", msg.MessageID())
}

func readConfig() *config {

	file, err := os.Open("config.json")
	if err != nil {
		panic(fmt.Sprintln("error:", err))
	}
	decoder := json.NewDecoder(file)
	result := config{}
	err = decoder.Decode(&result)
	if err != nil {
		panic(fmt.Sprintln("error:", err))
	}
	return &result
}

func sendMessagesFromChannel(api *slack.Client, config *config, params slack.PostMessageParameters) {
	for {
		api.PostMessage(config.Channel, <-postMessagesChannel, params)
	}
}

func doSlackAPI(config *config, wgReady *sync.WaitGroup) {
	var channelID string
	//setup
	params := slack.PostMessageParameters{}
	params.AsUser = true

	api := slack.New(config.Secret)

	logger := log.New(os.Stdout, "toolbot ", log.Lshortfile|log.LstdFlags)
	slack.SetLogger(logger)
	api.SetDebug(config.Debug)

	channels, _ := api.GetChannels(true)
	for _, channel := range channels {
		if channel.Name == config.Channel[1:] { //strip leading #
			channelID = channel.ID
		}
	}

	go sendMessagesFromChannel(api, config, params)

	//Bot is online
	wgReady.Done()

	//RTM
	go doSlackRTM(api, config, channelID)
}

func doSlackRTM(api *slack.Client, config *config, channelID string) {
	var botID string
	rtm := api.NewRTM()
	go rtm.ManageConnection()

	for message := range rtm.IncomingEvents {
		switch event := message.Data.(type) {
		case *slack.ConnectedEvent:
			botID = event.Info.User.ID

		case *slack.MessageEvent:
			if config.Debug {
				fmt.Printf("Message: %v\n", event.Msg.Text)
			}
			if (event.Msg.Channel == channelID) && (event.Msg.User != botID) { //Filtering for channel and own messages
				messagesChannel <- event.Msg.Text
			}

		case *slack.RTMError:
			panic(fmt.Sprintf("Error: %s\n", event.Error()))

		case *slack.InvalidAuthEvent:
			panic(fmt.Sprintf("Invalid credentials"))

		default:
			// Ignore everything else
		}
	}
}

func doMQTT(config *config, wgReady *sync.WaitGroup) {
	if config.Debug {
		mqtt.DEBUG = log.New(os.Stdout, "", 0)
	}
	mqtt.ERROR = log.New(os.Stdout, "", 0)
	mqttOptions := mqtt.NewClientOptions().AddBroker("tcp://" + config.Broker + ":" + config.Port).SetClientID(config.ClientID)
	mqttOptions.SetKeepAlive(10 * time.Second)
	mqttOptions.SetAutoReconnect(true)
	mqttOptions.SetMessageChannelDepth(messageChannelDepth)

	mqttClient := mqtt.NewClient(mqttOptions)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	//Subscribe
	if token := mqttClient.Subscribe(config.Topic, 2, handleSubscribedInboundMQTT); token.Wait() && token.Error() != nil {
		panic(fmt.Sprintln(token.Error()))
	}

	//working
	wgReady.Done()

	//sending the messages
	for {
		token := mqttClient.Publish(config.Topic, 2, false, <-messagesChannel)
		pubToken, _ := token.(*mqtt.PublishToken)
		fmt.Printf("MID: %d", pubToken.MessageID())
		if !token.WaitTimeout(20000) {
			fmt.Println("Timeout waiting to publish")
		}
	}
}

func main() {
	messagesChannel = make(chan string, channelBufferSize)
	postMessagesChannel = make(chan string, channelBufferSize)
	control = make(chan bool)
	var wgReady sync.WaitGroup
	wgReady.Add(2) //2 Connectors: Slack and MQTT

	//Config
	config := readConfig()
	if config.Debug {
		fmt.Println(config.Secret)
	}

	//SLACK
	go doSlackAPI(config, &wgReady)

	//MQTT
	go doMQTT(config, &wgReady)

	wgReady.Wait() //wait till both Connector-Routines report ready
	//Report Ready
	fmt.Println("Ready")

	//block execution
	<-control
}
