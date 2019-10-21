package clusterresourceoverride

type Config struct {
	LimitCPUToMemoryRatio     float64
	CpuRequestToLimitRatio    float64
	MemoryRequestToLimitRatio float64
}

