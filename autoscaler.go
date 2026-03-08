package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/pgpandit1008/network-aware-autoscaler/pkg/metrics"
	"github.com/pgpandit1008/network-aware-autoscaler/pkg/scaler"
)

// ScalingThresholds defines the thresholds that trigger scale-up/down
type ScalingThresholds struct {
	CPUPercent    float64
	MemoryPercent float64
	P95LatencyMs  float64
}

// AutoscalerController watches deployments and scales based on Prometheus metrics
type AutoscalerController struct {
	clientset     kubernetes.Interface
	metricsClient *metrics.PrometheusClient
	namespace     string
	thresholds    ScalingThresholds
}

// NewAutoscalerController creates a new AutoscalerController
func NewAutoscalerController(
	clientset kubernetes.Interface,
	metricsClient *metrics.PrometheusClient,
	namespace string,
	thresholds ScalingThresholds,
) *AutoscalerController {
	return &AutoscalerController{
		clientset:     clientset,
		metricsClient: metricsClient,
		namespace:     namespace,
		thresholds:    thresholds,
	}
}

// Run starts the autoscaler control loop
func (c *AutoscalerController) Run(interval time.Duration) {
	klog.Infof("Starting autoscaler control loop (interval: %s, namespace: %s)", interval, c.namespace)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.reconcile(); err != nil {
				klog.Errorf("Reconciliation error: %v", err)
			}
		}
	}
}

// reconcile lists all deployments and evaluates scaling decisions
func (c *AutoscalerController) reconcile() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	deployments, err := c.clientset.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "network-autoscaler=enabled",
	})
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	klog.V(3).Infof("Found %d autoscaler-enabled deployments in namespace %s", len(deployments.Items), c.namespace)

	for _, dep := range deployments.Items {
		if err := c.evaluateDeployment(ctx, &dep); err != nil {
			klog.Errorf("Failed to evaluate deployment %s: %v", dep.Name, err)
		}
	}
	return nil
}

// evaluateDeployment fetches metrics and scales if thresholds are breached
func (c *AutoscalerController) evaluateDeployment(ctx context.Context, dep *appsv1.Deployment) error {
	snapshot, err := c.metricsClient.GetMetrics(ctx, c.namespace, dep.Name)
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}

	currentReplicas := *dep.Spec.Replicas
	decision := scaler.Evaluate(snapshot, c.thresholds.CPUPercent, c.thresholds.MemoryPercent, c.thresholds.P95LatencyMs, currentReplicas)

	if decision.DesiredReplicas == currentReplicas {
		klog.V(4).Infof("Deployment %s is stable at %d replicas (CPU=%.1f%% MEM=%.1f%% P95=%.1fms)",
			dep.Name, currentReplicas, snapshot.CPUPercent, snapshot.MemoryPercent, snapshot.P95LatencyMs)
		return nil
	}

	klog.Infof("Scaling %s: %d -> %d replicas | Reason: %s | CPU=%.1f%% MEM=%.1f%% P95=%.1fms",
		dep.Name, currentReplicas, decision.DesiredReplicas, decision.Reason,
		snapshot.CPUPercent, snapshot.MemoryPercent, snapshot.P95LatencyMs)

	dep.Spec.Replicas = &decision.DesiredReplicas
	_, err = c.clientset.AppsV1().Deployments(c.namespace).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment replicas: %w", err)
	}

	klog.Infof("Successfully scaled %s to %d replicas", dep.Name, decision.DesiredReplicas)
	return nil
}
