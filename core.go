package prom

import (
	"time"

	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

type Config struct {
	Ip           string
	Port         string
	QueryTimeout int
}

type Prometheus struct {
	Client       api.Client
	QueryTimeout time.Duration
}

type Metric map[LabelName]LabelValue
type LabelName string
type LabelValue string
type LabelCondition map[string]string
type ValueCondition string

// Index is the index name of Prometheus
type Index struct {
	Metrics []Metric
	Value   float64
}

func (p *Prometheus) NewApi() *Api {
	return &Api{
		api:          prometheusv1.NewAPI(p.Client),
		QueryTimeout: p.QueryTimeout,
	}
}

func NewDefaultPrometheus() (*Prometheus, error) {
	cli, err := api.NewClient(api.Config{
		Address: "http://10.10.0.16:9090",
	})
	if err != nil {
		return nil, err
	}
	return &Prometheus{
		Client:       cli,
		QueryTimeout: time.Second * 5,
	}, nil
}

func NewPrometheus(config *Config) (*Prometheus, error) {
	cli, err := api.NewClient(api.Config{
		Address: "http://" + config.Ip + ":" + config.Port,
	})
	if err != nil {
		return nil, err
	}
	return &Prometheus{
		Client:       cli,
		QueryTimeout: time.Duration(config.QueryTimeout) * time.Second,
	}, nil
}
