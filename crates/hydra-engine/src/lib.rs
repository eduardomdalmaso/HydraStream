pub mod shm;
pub mod governor;
pub mod pipeline;
pub mod transport;
pub mod sync;
pub mod codec;
pub mod gateway;
pub mod actor;
pub mod coroutine;
pub mod resilience;
pub mod decorator;
pub mod isolated;
pub mod ffi;
pub mod wsp;
pub mod transcode;
pub mod server;
pub mod rtmp;

pub use shm::{ShmWriter, ShmReader, ShmHeader, SlotHeader};
pub use governor::FpsGovernor;
pub use pipeline::{StreamPipeline, StreamPipelineBuilder};
pub use transport::{
    BroadcastHub, FramePayload, FrameStorage, LockFreePeerList, RcuConfig,
    CacheAlignedAtomicU64, CacheAlignedAtomicU32,
    BIT_WEBSOCKET, BIT_WEBRTC, BIT_ANALYTICS, BIT_ALL,
};
pub use sync::{AtomicSemaphore, HybridMutex, ParkingTable};
pub use codec::{NalParser, NalType, GopTracker, FLAG_KEYFRAME, FLAG_SPS_PPS, FLAG_DELTA_FRAME, FLAG_GPU_SURFACE};
pub use gateway::{
    AsyncStreamGateway, ClientSession, StreamError,
    LocalSessionCache, GatewayRuntimeBuilder, spawn_blocking_shm_bridge,
};
pub use actor::{GopActorHandle, GopStats};
pub use coroutine::{SymmetricCoroutine, CoroutineYield, NalClassifierCoroutine, FpsFilterCoroutine};
pub use resilience::{StreamCircuitBreaker, CircuitState};
pub use decorator::{TelemetryFuture, TelemetryExt, FutureMetricsSink};
pub use isolated::{SyncGatewayBridge, SyncReceiver};
pub use wsp::{pack_wsp_frame, pack_wsp_frame_into, unpack_wsp_frame, WspFrameRef, WSP_FRAME_KEYFRAME, WSP_FRAME_DELTA, WSP_HEADER_LEN};
pub use transcode::{TranscodeWorker, TranscodePool, TranscodeError, detect_nvidia_gpu};
pub use server::{WspServer, StreamRegistry, StreamCodec, StreamEntry};
pub use rtmp::{RtmpServer, rtmp_handshake, convert_avcc_to_annexb, extract_sps_pps_from_avcc};
