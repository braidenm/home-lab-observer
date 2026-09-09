package history

import "github.com/braidenm/home-lab-observer/internal/observation"

func Extract(snapshot observation.Snapshot, previous *observation.Snapshot) []Sample {
	at := snapshot.ObservedAt.UTC()
	result := make([]Sample, 0, len(allowedMetrics))
	if usable(snapshot.CPU.State) && snapshot.CPU.Data != nil {
		result = appendValid(result, Sample{Metric: CPUUtilization, At: at, Value: snapshot.CPU.Data.UsagePercent})
	}
	if usable(snapshot.Memory.State) && snapshot.Memory.Data != nil {
		result = appendValid(result, Sample{Metric: MemoryUtilization, At: at, Value: snapshot.Memory.Data.UsagePercent})
	}
	if usable(snapshot.Filesystems.State) && snapshot.Filesystems.Data != nil {
		var used, total float64
		for _, filesystem := range *snapshot.Filesystems.Data {
			if filesystem.UsedBytes > filesystem.TotalBytes {
				continue
			}
			used += float64(filesystem.UsedBytes)
			total += float64(filesystem.TotalBytes)
		}
		if total > 0 {
			result = appendValid(result, Sample{Metric: FilesystemUtilization, At: at, Value: used * 100 / total})
		}
	}
	if usable(snapshot.Processes.State) && snapshot.Processes.Data != nil {
		count := snapshot.Processes.Quality.Total
		if count < len(*snapshot.Processes.Data) {
			count = len(*snapshot.Processes.Data)
		}
		result = appendValid(result, Sample{Metric: ProcessCount, At: at, Value: float64(count)})
	}
	if previous != nil && usable(snapshot.Network.State) && usable(previous.Network.State) && snapshot.Network.Data != nil && previous.Network.Data != nil {
		seconds := snapshot.ObservedAt.Sub(previous.ObservedAt).Seconds()
		if seconds > 0 && snapshot.Network.Data.BytesRecv >= previous.Network.Data.BytesRecv && snapshot.Network.Data.BytesSent >= previous.Network.Data.BytesSent {
			result = appendValid(result, Sample{Metric: NetworkReceiveRate, At: at, Value: float64(snapshot.Network.Data.BytesRecv-previous.Network.Data.BytesRecv) / seconds})
			result = appendValid(result, Sample{Metric: NetworkTransmitRate, At: at, Value: float64(snapshot.Network.Data.BytesSent-previous.Network.Data.BytesSent) / seconds})
		}
	}
	return result
}

func usable(state observation.SupportState) bool {
	return state == observation.Available || state == observation.Degraded
}

func appendValid(samples []Sample, sample Sample) []Sample {
	if sample.validate() == nil {
		return append(samples, sample)
	}
	return samples
}
