"""Data models for the VoiceAgent SDK."""

from dataclasses import dataclass, field
from typing import Optional


@dataclass
class Agent:
    id: str = ""
    name: str = ""
    email: str = ""
    phone: str = ""
    expertise: list[str] = field(default_factory=list)
    status: str = "available"
    max_calls: int = 3
    active_calls: int = 0


@dataclass
class Utterance:
    speaker: str = ""
    text: str = ""
    timestamp: str = ""


@dataclass
class Suggestion:
    suggestion: str = ""
    context: str = ""
    category: str = ""
    confidence: float = 0.0


@dataclass
class CallSummary:
    conversation_id: str = ""
    duration_seconds: int = 0
    summary: str = ""
    action_items: list[str] = field(default_factory=list)
    commitments_made: list[str] = field(default_factory=list)
    sentiment: str = "neutral"


@dataclass
class Call:
    id: str = ""
    caller_number: str = ""
    called_number: str = ""
    mode: str = ""
    status: str = ""
    duration: int = 0
    summary: str = ""
    sentiment: str = ""
    start_time: str = ""


@dataclass
class Document:
    id: str = ""
    name: str = ""
    type: str = ""
    size: int = 0
    category: str = ""
    chunks: int = 0
    status: str = ""


@dataclass
class RAGResult:
    text: str = ""
    doc_name: str = ""
    score: float = 0.0
    chunk_index: int = 0


@dataclass
class LLMConfig:
    id: str = ""
    name: str = ""
    provider: str = ""
    model: str = ""
    region: str = ""
    is_default: bool = False
    max_tokens: int = 512


@dataclass
class LLMTestResult:
    response: str = ""
    latency: int = 0
    model: str = ""


@dataclass
class RobocallResult:
    score: float = 0.0
    category: str = ""
    reason: str = ""
    keywords: list[str] = field(default_factory=list)
    blocked: bool = False


@dataclass
class PIIResult:
    original: str = ""
    masked: str = ""
    pii_found: bool = False
    detections: list[dict] = field(default_factory=list)


@dataclass
class VoicePrint:
    id: str = ""
    label: str = ""
    type: str = ""
    features_dim: int = 0


@dataclass
class FailoverStatus:
    llm: dict = field(default_factory=dict)
    stt: dict = field(default_factory=dict)
    tts: dict = field(default_factory=dict)
    esl: dict = field(default_factory=dict)


@dataclass
class DashboardStats:
    active_calls: int = 0
    total_today: int = 0
    avg_duration: int = 0
    sentiment_breakdown: dict = field(default_factory=dict)


@dataclass
class SSEEvent:
    type: str = ""
    speaker: Optional[str] = None
    text: Optional[str] = None
    suggestion: Optional[str] = None
    category: Optional[str] = None
    confidence: Optional[float] = None
    summary: Optional[str] = None
    action_items: Optional[list[str]] = None
    sentiment: Optional[str] = None
    duration: Optional[int] = None
