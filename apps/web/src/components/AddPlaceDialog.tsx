import { useMutation } from "@tanstack/react-query";
import { MapPin, X } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { createPlace } from "../lib/api";
import type { PlaceView } from "../types";

const placeTypeOptions = [
  { value: "city", label: "مدينة" },
  { value: "region", label: "منطقة" },
  { value: "village", label: "قرية" },
  { value: "historical_settlement", label: "موضع تاريخي" },
  { value: "other", label: "أخرى" },
];

interface AddPlaceDialogProps {
  open: boolean;
  onClose: () => void;
  onCreated: (place: PlaceView) => void;
}

/**
 * Adds a place as a private research draft. The backend stores it private; the
 * dialog says so instead of implying it joins the public map. Longitude and
 * latitude are all-or-nothing because a place with one coordinate has no position.
 */
export function AddPlaceDialog({ open, onClose, onCreated }: AddPlaceDialogProps) {
  const [name, setName] = useState("");
  const [placeType, setPlaceType] = useState("city");
  const [notes, setNotes] = useState("");
  const [longitude, setLongitude] = useState("");
  const [latitude, setLatitude] = useState("");
  const [error, setError] = useState("");
  const nameRef = useRef<HTMLInputElement>(null);
  const mutation = useMutation({
    mutationFn: () => createPlace({
      canonical_name_ar: name.trim(),
      place_type: placeType,
      notes_ar: notes.trim() || undefined,
      longitude: longitude.trim() === "" ? undefined : Number(longitude),
      latitude: latitude.trim() === "" ? undefined : Number(latitude),
    }),
    onSuccess: (place) => {
      onCreated(place);
      onClose();
    },
    onError: (reason) => setError(reason instanceof Error ? reason.message : "تعذر إضافة الموضع."),
  });

  useEffect(() => {
    if (!open) return;
    setName("");
    setPlaceType("city");
    setNotes("");
    setLongitude("");
    setLatitude("");
    setError("");
    mutation.reset();
    const frame = requestAnimationFrame(() => nameRef.current?.focus());
    return () => cancelAnimationFrame(frame);
    // The form is reset only when the dialog opens. `mutation` is a stable
    // object from React Query and is deliberately not a dependency.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open) return undefined;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open, onClose]);

  if (!open) return null;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setError("");
    if (name.trim().length === 0) {
      setError("اكتب اسم الموضع.");
      return;
    }
    const hasLongitude = longitude.trim() !== "";
    const hasLatitude = latitude.trim() !== "";
    if (hasLongitude !== hasLatitude) {
      setError("أدخل خط الطول وخط العرض معاً، أو اتركهما فارغين.");
      return;
    }
    if (hasLongitude) {
      const longitudeValue = Number(longitude);
      const latitudeValue = Number(latitude);
      if (!Number.isFinite(longitudeValue) || longitudeValue < -180 || longitudeValue > 180) {
        setError("خط الطول يجب أن يكون رقماً بين ‎-180‎ و‎180‎.");
        return;
      }
      if (!Number.isFinite(latitudeValue) || latitudeValue < -90 || latitudeValue > 90) {
        setError("خط العرض يجب أن يكون رقماً بين ‎-90‎ و‎90‎.");
        return;
      }
    }
    mutation.mutate();
  };

  return (
    <div className="atlas-dialog-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <div className="atlas-dialog" role="dialog" aria-modal="true" aria-labelledby="atlas-add-place-title" aria-describedby="atlas-add-place-description">
        <div className="atlas-dialog-head">
          <div>
            <div className="eyebrow">فهرس المواضع</div>
            <h2 id="atlas-add-place-title">أضف موضعاً إلى بحثك</h2>
          </div>
          <button type="button" className="atlas-inspector-close" onClick={onClose} aria-label="أغلق نافذة إضافة موضع">
            <X size={16} />
          </button>
        </div>
        <p id="atlas-add-place-description" className="atlas-dialog-note">
          يُحفظ الموضع كمسودة خاصة بك، ولا يظهر على الخريطة العامة. الإحداثيات اختيارية، لكنها إن وُجدت تظهر عليه.
        </p>
        <form className="atlas-dialog-form" onSubmit={submit}>
          <label>
            <span>الاسم</span>
            <input ref={nameRef} value={name} onChange={(event) => setName(event.target.value)} placeholder="مثال: قرية النخيل" required />
          </label>
          <label>
            <span>النوع</span>
            <select value={placeType} onChange={(event) => setPlaceType(event.target.value)}>
              {placeTypeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </label>
          <label className="atlas-dialog-wide">
            <span>ملاحظات</span>
            <textarea value={notes} onChange={(event) => setNotes(event.target.value)} rows={3} placeholder="ما يعرفه الباحث عن هذا الموضع، وما لم يُحسم بعد." />
          </label>
          <label>
            <span>خط الطول</span>
            <input inputMode="decimal" value={longitude} onChange={(event) => setLongitude(event.target.value)} placeholder="46.6753" />
          </label>
          <label>
            <span>خط العرض</span>
            <input inputMode="decimal" value={latitude} onChange={(event) => setLatitude(event.target.value)} placeholder="24.7136" />
          </label>
          {error ? <p className="atlas-dialog-error" role="alert">{error}</p> : null}
          <div className="atlas-dialog-actions">
            <button type="button" className="secondary-button" onClick={onClose}>إلغاء</button>
            <button type="submit" className="primary-button" disabled={mutation.isPending}>
              <MapPin size={15} />
              {mutation.isPending ? "جارٍ الحفظ…" : "أضف الموضع"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
