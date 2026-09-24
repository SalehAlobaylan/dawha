import {
  AlertTriangle,
  FileText,
  GitBranch,
  Lightbulb,
  MessageCircleQuestion,
  ScanSearch,
} from "lucide-react";
import type { ComponentType } from "react";
import type { EpistemicTone } from "../types";

interface StatusBadgeProps {
  tone: EpistemicTone;
  children: React.ReactNode;
  compact?: boolean;
}

const toneConfig: Record<EpistemicTone, { label: string; icon: ComponentType<{ size?: number; strokeWidth?: number }> }> = {
  source: { label: "مصدر", icon: FileText },
  claim: { label: "ادعاء", icon: ScanSearch },
  interpretation: { label: "تفسير", icon: GitBranch },
  finding: { label: "ملاحظة النظام", icon: Lightbulb },
  question: { label: "سؤال مفتوح", icon: MessageCircleQuestion },
  disputed: { label: "متنازع عليه", icon: AlertTriangle },
};

export function StatusBadge({ tone, children, compact = false }: StatusBadgeProps) {
  const config = toneConfig[tone];
  const Icon = config.icon;

  return (
    <span className={`status-badge status-${tone}${compact ? " status-compact" : ""}`}>
      <Icon size={compact ? 12 : 13} strokeWidth={1.8} />
      <span>{children}</span>
    </span>
  );
}
