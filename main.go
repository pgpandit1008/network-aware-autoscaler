package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/pgpandit1008/network-aware-autoscaler/pkg/controller"
	"github.com/pgpandit1008/network-aware-autoscaler/pkg/metrics"
)

func main() {
	var (
		kubeconfig      string
		prometheusURL   string
		syncInterval    time.Duration
		cpuThreshold    float64
		memThreshold    float64
		p95Threshold    float64
		namespace       string
	)

	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (leave empty for in-cluster)")
	flag.StringVar(&prometheusURL, "prometheus-url", "http://prometheus:9090", "Prometheus server URL")
	flag.DurationVar(&syncInterval, "sync-interval", 30*time.Second, "Interval between scaling evaluations")
	flag.Float64Var(&cpuThreshold, "cpu-threshold", 75.0, "CPU utilization threshold (%) to trigger scale-up")
	flag.Float64Var(&memThreshold, "mem-threshold", 80.0, "Memory utilization threshold (%) to trigger scale-up")
	flag.Float64Var(&p95Threshold, "p95-threshold", 200.0, "p95 latency threshold (ms) to trigger scale-up")
	flag.StringVar(&namespace, "namespace", "default", "Kubernetes namespace to watch")
	flag.Parse()

	klog.InitFlags(nil)
	klog.Infof("Starting Network-Aware Kubernetes Autoscaler")
	klog.Infof("Prometheus URL: %s", prometheusURL)
	klog.Infof("Sync interval: %s", syncInterval)

	// Build kubeconfig
	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		klog.Errorf("Failed to build kubeconfig: %v", err)
		os.Exit(1)
	}

	// Build kubernetes client
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("Failed to create Kubernetes client: %v", err)
		os.Exit(1)
	}

	// Initialize Prometheus metrics client
	metricsClient := metrics.NewPrometheusClient(prometheusURL)

	// Initialize and run controller
	ctrl := controller.NewAutoscalerController(
		clientset,
		metricsClient,
		namespace,
		controller.ScalingThresholds{
			CPUPercent:    cpuThreshold,
			MemoryPercent: memThreshold,
			P95LatencyMs:  p95Threshold,
		},
	)

	ctrl.Run(syncInterval)
}
