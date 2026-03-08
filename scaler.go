package scaler

import "fmt"

// ScalingDecision holds the result of a scaling evaluation
type ScalingDecision struct {
	DesiredReplicas int32
	Reason          string
	ScaleUp         bool
}

const (
	maxReplicas     int32 = 10
	minReplicas     int32 = 1
	scaleUpStep     int32 = 2
	scaleDownStep   int32 = 1
	scaleDownFactor       = 0.5 // scale down only if all metrics are below 50% of threshold
)

// Evaluate determines the desired replica count based on current metrics and thresholds
func Evaluate(snapshot interface{ GetCPU() float64; GetMemory() float64; GetP95() float64 }, cpuThreshold, memThreshold, p95Threshold float64, current int32) ScalingDecision {
	cpu := snapshot.GetCPU()
	mem := snapshot.GetMemory()
	p95 := snapshot.GetP95()

	// Scale up if ANY metric exceeds threshold
	if cpu > cpuThreshold {
		desired := min32(current+scaleUpStep, maxReplicas)
		return ScalingDecision{
			DesiredReplicas: desired,
			Reason:          fmt.Sprintf("CPU %.1f%% > threshold %.1f%%", cpu, cpuThreshold),
			ScaleUp:         true,
		}
	}

	if mem > memThreshold {
		desired := min32(current+scaleUpStep, maxReplicas)
		return ScalingDecision{
			DesiredReplicas: desired,
			Reason:          fmt.Sprintf("Memory %.1f%% > threshold %.1f%%", mem, memThreshold),
			ScaleUp:         true,
		}
	}

	if p95 > p95Threshold {
		desired := min32(current+scaleUpStep, maxReplicas)
		return ScalingDecision{
			DesiredReplicas: desired,
			Reason:          fmt.Sprintf("p95 latency %.1fms > threshold %.1fms", p95, p95Threshold),
			ScaleUp:         true,
		}
	}

	// Scale down only if ALL metrics are well below threshold
	cpuLow := cpu < cpuThreshold*scaleDownFactor
	memLow := mem < memThreshold*scaleDownFactor
	p95Low := p95 < p95Threshold*scaleDownFactor

	if cpuLow && memLow && p95Low && current > minReplicas {
		desired := max32(current-scaleDownStep, minReplicas)
		return ScalingDecision{
			DesiredReplicas: desired,
			Reason:          fmt.Sprintf("All metrics below 50%% of thresholds (CPU=%.1f%% MEM=%.1f%% P95=%.1fms)", cpu, mem, p95),
			ScaleUp:         false,
		}
	}

	return ScalingDecision{DesiredReplicas: current, Reason: "stable"}
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
