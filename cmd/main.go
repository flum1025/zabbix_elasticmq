package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/adubkov/go-zabbix"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type config struct {
	queueEndpoint              string
	zabbixHost                 string
	zabbixPort                 int
	zabbixTargetHost           string
	zabbixAutoDiscoveryKeyName string
	zabbixItemKeyName          string
	interval                   int
}

func getEnvString(param string, defaultValue string) string {
	val, ok := os.LookupEnv(param)

	if !ok {
		return defaultValue
	}

	return val
}

func getEnvInt(param string, defaultValue int) int {
	val, ok := os.LookupEnv(param)

	if !ok {
		return defaultValue
	}

	v, err := strconv.Atoi(val)

	if err != nil {
		panic(err.Error())
	}

	return v
}

func newConfig() *config {
	return &config{
		queueEndpoint:              getEnvString("QUEUE_ENDPOINT", ""),
		zabbixHost:                 getEnvString("ZABBIX_HOST", ""),
		zabbixPort:                 getEnvInt("ZABBIX_PORT", 10051),
		zabbixTargetHost:           getEnvString("ZABBIX_TARGET_HOST", ""),
		zabbixAutoDiscoveryKeyName: getEnvString("ZABBIX_AUTO_DISCOVERY_KEY_NAME", "elasticmq.queue.discovery"),
		zabbixItemKeyName:          getEnvString("ZABBIX_ITEM_KEY_NAME", "elasticmq.queue"),
		interval:                   getEnvInt("INTERVAL", 300),
	}
}

type sqsClient struct {
	sqs *sqs.SQS
}

func newSqs(config *config) *sqsClient {
	session := session.Must(session.NewSession(
		&aws.Config{
			Region: aws.String("us-west-2"),
			EndpointResolver: endpoints.ResolverFunc(
				func(string, string, ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
					return endpoints.ResolvedEndpoint{
						URL: config.queueEndpoint,
					}, nil
				},
			),
		},
	))

	return &sqsClient{
		sqs: sqs.New(session),
	}
}

func (c *sqsClient) listQueues() (map[string]*string, error) {
	res, err := c.sqs.ListQueues(nil)

	if err != nil {
		return nil, err
	}

	m := map[string]*string{}

	for _, url := range res.QueueUrls {
		m[filepath.Base(*url)] = url
	}

	return m, nil
}

func (c *sqsClient) getQueueAttributes(url *string) (map[string]*string, error) {
	toPtr := func(s string) *string {
		return &s
	}

	res, err := c.sqs.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueUrl: url,
		AttributeNames: []*string{
			toPtr("ApproximateNumberOfMessages"),
			toPtr("ApproximateNumberOfMessagesDelayed"),
			toPtr("ApproximateNumberOfMessagesNotVisible"),
		},
	})

	if err != nil {
		return nil, err
	}

	return res.Attributes, nil
}

type autoDiscoveryItem struct {
	Queue string `json:"{#QUEUE}"`
	Item  string `json:"{#ITEM}"`
}

type autoDiscoveryData struct {
	Data []autoDiscoveryItem `json:"data"`
}

type monitor struct {
	config *config
	sqs    *sqsClient
	sender *zabbix.Sender
	queues map[string]*string
}

func newMonitor(config *config) (*monitor, error) {
	sqs := newSqs(config)
	sender := zabbix.NewSender(config.zabbixHost, config.zabbixPort)
	queues, err := sqs.listQueues()

	if err != nil {
		return nil, err
	}

	return &monitor{
		config: config,
		sqs:    sqs,
		sender: sender,
		queues: queues,
	}, nil
}

func (m *monitor) autoDiscovery() (*string, error) {
	var metrics []*zabbix.Metric

	for name, url := range m.queues {
		attrs, err := m.sqs.getQueueAttributes(url)

		if err != nil {
			return nil, fmt.Errorf("get attribues: %w", err)
		}

		for attr := range attrs {
			json, _ := json.Marshal(
				autoDiscoveryData{
					Data: []autoDiscoveryItem{
						{
							Queue: name,
							Item:  attr,
						},
					},
				},
			)

			metrics = append(metrics, zabbix.NewMetric(
				m.config.zabbixTargetHost,
				m.config.zabbixAutoDiscoveryKeyName,
				string(json),
			))
		}
	}

	return m.send(metrics)
}

func (m *monitor) exec() (*string, error) {
	var metrics []*zabbix.Metric

	for name, url := range m.queues {
		attrs, err := m.sqs.getQueueAttributes(url)

		if err != nil {
			return nil, fmt.Errorf("get attribues: %w", err)
		}

		for attr, value := range attrs {
			metrics = append(metrics, zabbix.NewMetric(
				m.config.zabbixTargetHost,
				fmt.Sprintf("%s[%s,%s]", m.config.zabbixItemKeyName, name, attr),
				*value,
			))
		}
	}

	return m.send(metrics)
}

func (m *monitor) send(metrics []*zabbix.Metric) (*string, error) {
	packet := zabbix.NewPacket(metrics)
	res, err := m.sender.Send(packet)

	if err != nil {
		return nil, fmt.Errorf("send zabbix: %w", err)
	}

	r := string(res)

	return &r, nil
}

func main() {
	config := newConfig()
	monitor, err := newMonitor(config)

	if err != nil {
		log.Fatalf("failed to initialize monitor: %v", err)
	}

	res, err := monitor.autoDiscovery()

	if err != nil {
		log.Fatalf("failed to send autoDiscovery: %v", err)
	}

	log.Printf("send autoDiscovery: %v", *res)

	exec := func() {
		res, err := monitor.exec()

		if err != nil {
			log.Fatalf("failed to send monitoring data: %v", err)
		}

		log.Printf("send monitoring data: %v", *res)
	}

	exec()

	ticker := time.NewTicker(time.Duration(config.interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			exec()
		}
	}
}
