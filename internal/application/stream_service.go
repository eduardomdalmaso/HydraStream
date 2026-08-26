package application

import (
	"context"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// StreamService is the application service handling stream use cases.
type StreamService struct {
	repo ports.StreamRepository
}

// NewStreamService creates a new StreamService application instance.
func NewStreamService(repo ports.StreamRepository) *StreamService {
	return &StreamService{repo: repo}
}

func (s *StreamService) RegisterStream(ctx context.Context, st *domain.Stream) error {
	if err := st.Validate(); err != nil {
		return err
	}
	st.SetDefaults()
	return s.repo.Save(ctx, st)
}

func (s *StreamService) GetStream(ctx context.Context, streamID string) (*domain.Stream, error) {
	if streamID == "" {
		return nil, domain.ErrInvalidStream
	}
	return s.repo.FindByID(ctx, streamID)
}

func (s *StreamService) ListStreams(ctx context.Context, searchQuery, tenantFilter, sortBy string, page, limit int) ([]*domain.Stream, int, error) {
	return s.repo.ListFiltered(ctx, searchQuery, tenantFilter, sortBy, page, limit)
}

func (s *StreamService) DeleteStream(ctx context.Context, streamID string) error {
	if streamID == "" {
		return domain.ErrInvalidStream
	}
	return s.repo.Delete(ctx, streamID)
}

func (s *StreamService) UpdateConsumer(ctx context.Context, streamID, analyticType string, targetFPS float64, format string) error {
	if streamID == "" || analyticType == "" {
		return domain.ErrInvalidStream
	}
	return s.repo.UpdateConsumerFPS(ctx, streamID, analyticType, targetFPS, format)
}

func (s *StreamService) GetClusterTopology(ctx context.Context, streamID string) (*domain.ClusterTopology, error) {
	if streamID == "" {
		streamID = "cam_entrance_01"
	}
	topo := &domain.ClusterTopology{
		StreamID: streamID,
		IngestionNode: domain.TopologyNode{
			NodeName:      "k8s-gpu-node-02",
			NodeIP:        "10.0.1.45",
			CPUArch:       "AMD EPYC 7763 64-Core",
			GPUHardware:   "NVIDIA A100-SXM4-80GB",
			DecoderEngine: "NVDEC (GPU 0)",
		},
		ConsumerRoute: []domain.ConsumerRouting{
			{
				Analytic:       "yolo_detection",
				TargetNode:     "k8s-gpu-node-02",
				SameNode:       true,
				TransportUsed:  "CUDA_IPC (Zero-Copy VRAM Direct)",
				TargetHardware: "NVIDIA A100-SXM4-80GB",
			},
		},
	}
	return topo, nil
}

func (s *StreamService) GetSystemInfo(ctx context.Context) (*domain.SystemInfo, error) {
	uptime := s.repo.UptimeSeconds(ctx)
	info := &domain.SystemInfo{
		AppName:       "HydraStream Engine",
		Version:       "1.0.0",
		UptimeSeconds: uptime,
		EngineMode:    "nvidia_nvdec",
		GPUDetected:   true,
		GPUModel:      "NVIDIA RTX 4090",
		Features: map[string]bool{
			"posix_shm":   true,
			"cuda_ipc":    true,
			"triton_grpc": true,
		},
	}
	return info, nil
}

// Ensure interface compliance
var _ ports.StreamUseCase = (*StreamService)(nil)
