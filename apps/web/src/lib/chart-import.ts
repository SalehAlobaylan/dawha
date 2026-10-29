export const MAX_CHART_BYTES = 128 * 1024 * 1024;

export function chartFileError(file: Pick<File, "name" | "type" | "size">): string | null {
  const pdf = file.type === "application/pdf" || ((!file.type || file.type === "application/octet-stream") && /\.pdf$/i.test(file.name));
  if (!pdf) return "اختر ملف PDF للمشجر. يمكنك إضافة أنواع المصادر الأخرى من صفحة المصادر.";
  if (file.size === 0) return "الملف فارغ. اختر نسخة أخرى من المشجر.";
  if (file.size > MAX_CHART_BYTES) return "حجم الملف يتجاوز 128 ميغابايت. اختر نسخة أصغر.";
  return null;
}
