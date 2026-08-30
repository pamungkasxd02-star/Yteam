"""LocalSolver: safe anti-bot observation for authorized recon."""

from .detector import GATE_MARKERS, GateKind, LocalSolver, LocalSolverDecision, classify_response, gate_summary
from .camoufox_adapter import CamoufoxConfig, run_camoufox, safe_url, same_origin

__all__ = [
    "GATE_MARKERS", "GateKind", "LocalSolver", "LocalSolverDecision",
    "classify_response", "gate_summary", "CamoufoxConfig", "run_camoufox",
    "safe_url", "same_origin",
]
