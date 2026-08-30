"""Bot-bypass pillar for authorized penetration testing and QA.

Purpose: understand, classify, and — within written authorization — navigate
bot/anti-bot detection to test the application the way a real (non-bot) user
would. This is NOT for abuse, scraping-at-scale, spam, or bypassing protections
against a service you do not own or are not authorized to test.
"""

from .camoufox_adapter import CamoufoxConfig, run_camoufox, same_origin
from .detector import BOT_MARKERS, Botterdop, BotterdopDecision, GateKind, classify_response, gate_summary

__all__ = ["GateKind", "Botterdop", "BotterdopDecision", "CamoufoxConfig", "classify_response", "gate_summary", "run_camoufox", "same_origin", "BOT_MARKERS"]
