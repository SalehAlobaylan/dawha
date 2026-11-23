import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Maximize2, Minimize2, X } from "lucide-react";
import { useState } from "react";
import { fetchAtlasPlatePointCrop, type AtlasPlateRegisterRow } from "../lib/api";

export function PlateComparePanel({ register, longitude, latitude, onClose, floating = false }: {
  register: AtlasPlateRegisterRow[];
  longitude: number;
  latitude: number;
  onClose: () => void;
  floating?: boolean;
}) {
  return (
    <aside
      className={floating ? "plate-caption-panel plate-caption-panel-floating" : "atlas-inspector plate-caption-panel"}
      aria-label="مقارنة النقطة بين اللوحات"
    >
      <div className="atlas-panel-head plate-caption-head">
        <div className="plate-caption-title-group">
          <div className="eyebrow">مقارنة جغرافية</div>
          <h2>العثور على النقطة في كل لوحة</h2>
          <p className="plate-compare-point">{latitude.toFixed(3)}°N ، {longitude.toFixed(3)}°E</p>
        </div>
        <button type="button" className="atlas-inspector-close" onClick={onClose} aria-label="إغلاق المقارنة" title="إغلاق المقارنة">
          <X size={15} />
        </button>
      </div>
      {register.length === 0 && <p className="plate-caption-sources-status">لا لوحات مرجعية متاحة.</p>}
      <ul className="plate-compare-rows">
        {register.map((plate) => (
          <li className="plate-compare-row" key={plate.plateKey}>
            <strong className="plate-compare-row-title">{plate.titleAr || plate.plateKey}</strong>
            {plate.transform
              ? <PlatePointRead plate={plate} longitude={longitude} latitude={latitude} />
              : <p className="plate-compare-disabled-caption">{plate.disabledCaption}</p>}
          </li>
        ))}
      </ul>
    </aside>
  );
}

export function PlatePointRead({ plate, longitude, latitude, size }: {
  plate: AtlasPlateRegisterRow;
  size?: number;
  longitude: number;
  latitude: number;
}) {
  const [expanded, setExpanded] = useState(false);
  const crop = useQuery({
    queryKey: ["plate-point-crop", plate.plateKey, longitude, latitude, size],
    queryFn: ({ signal }) => fetchAtlasPlatePointCrop(plate.plateKey, longitude, latitude, signal, size),
    retry: false,
  });
  if (crop.isPending) {
    return (
      <div className="plate-crop-placeholder" role="status">
        جارٍ تحميل قصاصة اللوحة…
      </div>
    );
  }
  if (crop.isError) {
    return (
      <div className="plate-crop-error" role="alert">
        <span>تعذّر تحميل القصاصة.</span>
        <button type="button" className="text-button" onClick={() => void crop.refetch()}>أعد المحاولة</button>
      </div>
    );
  }
  if (!crop.data.insideNeatline) {
    return <p className="plate-caption-sources-status">النقطة خارج إطار اللوحة.</p>;
  }
  return (
    <figure className="plate-crop-card">
      <div className={`plate-crop-frame${expanded ? " plate-crop-frame-expanded" : ""}`}>
        <img
          src={`data:image/png;base64,${crop.data.cropPng}`}
          alt="قصاصة اللوحة عند الموضع المحدد"
          className="plate-compare-image"
        />
        <button
          type="button"
          className="plate-crop-zoom-btn"
          aria-pressed={expanded}
          title={expanded ? "تصغير القصاصة" : "تكبير القصاصة"}
          aria-label={expanded ? "تصغير القصاصة" : "تكبير القصاصة"}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? <Minimize2 size={13} /> : <Maximize2 size={13} />}
        </button>
      </div>
      <figcaption className={`plate-crop-status${crop.data.plate?.needsReview ? " plate-crop-status-warn" : ""}`}>
        <AlertTriangle size={12} aria-hidden="true" />
        <span>{crop.data.plate?.needsReview ? "المحاذاة تقريبية وبحاجة إلى مراجعة" : "المحاذاة تقريبية"}</span>
      </figcaption>
    </figure>
  );
}
