import brandMark from "../assets/brand/mark.png";

/**
 * The product mark. The artwork is the real brand, so it is an image rather than
 * a drawn stand-in: a rust three-figure genealogy mark on transparent, keyed out
 * of the master by apps/web/scripts/prepare-brand-assets.py.
 *
 * It is decorative — every place that renders it also renders the name "دَوْحة"
 * as text, so a screen reader announces the product rather than a file.
 */
export function BrandMark() {
  return (
    <span className="brand-mark">
      <img src={brandMark} alt="" width={28} height={32} decoding="async" />
    </span>
  );
}
