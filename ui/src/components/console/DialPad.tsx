"use client";

const KEYS = [
  ["1", "2", "3"],
  ["4", "5", "6"],
  ["7", "8", "9"],
  ["*", "0", "#"],
];

const SUB: Record<string, string> = {
  "2": "ABC",
  "3": "DEF",
  "4": "GHI",
  "5": "JKL",
  "6": "MNO",
  "7": "PQRS",
  "8": "TUV",
  "9": "WXYZ",
};

export function DialPad({
  visible,
  onDigit,
}: {
  visible: boolean;
  onDigit: (d: string) => void;
}) {
  if (!visible) return null;

  return (
    <div className="animate-[slide-up_0.3s_ease-out]">
      <div className="grid grid-cols-3 gap-1.5 mb-3">
        {KEYS.flat().map((k) => (
          <button
            key={k}
            onClick={() => onDigit(k)}
            className="h-12 rounded-lg bg-[#070b14] border border-white/[0.04] flex flex-col items-center justify-center transition-all duration-100 hover:bg-cyan-500/[0.08] active:bg-cyan-500/[0.15] active:scale-95"
          >
            <span className="text-lg font-mono text-slate-200">{k}</span>
            {SUB[k] && (
              <span className="text-[8px] font-mono text-slate-600 tracking-[0.2em]">
                {SUB[k]}
              </span>
            )}
          </button>
        ))}
      </div>
    </div>
  );
}
