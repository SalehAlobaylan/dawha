import type { SourceTreeSuggestion } from "../types";

// Both the crop-local rectangle and the page rectangle use the rendered image's
// own frame. CSS never places this over an object-fit letterbox.
export function SourceSpotMarker({ region, target = false }: { region: SourceTreeSuggestion["region"]; target?: boolean }) {
  const x = region.x + region.width / 2;
  const y = region.y + region.height / 2;
  return <div className="source-spot-marker" aria-hidden="true">
    <svg viewBox="0 0 1 1" preserveAspectRatio="none">
      {target ? <><line x1={0} y1={y} x2={1} y2={y} /><line x1={x} y1={0} x2={x} y2={1} /></> : null}
      <rect className="source-spot-halo" x={region.x} y={region.y} width={region.width} height={region.height} />
      <rect x={region.x} y={region.y} width={region.width} height={region.height} />
    </svg>
    {target ? <span className="source-spot-pin" style={{ left: `${x * 100}%`, top: `${y * 100}%` }} /> : null}
  </div>;
}
