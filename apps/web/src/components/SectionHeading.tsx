interface SectionHeadingProps {
  eyebrow: string;
  title: string;
  description?: string;
  action?: string;
  onAction?: () => void;
}

export function SectionHeading({ eyebrow, title, description, action, onAction }: SectionHeadingProps) {
  return (
    <div className="section-heading">
      <div>
        <div className="eyebrow">{eyebrow}</div>
        <h2>{title}</h2>
        {description ? <p>{description}</p> : null}
      </div>
      {action ? (
        <button className="text-button" type="button" onClick={onAction}>
          {action}
          <ArrowIcon />
        </button>
      ) : null}
    </div>
  );
}

function ArrowIcon() {
  return <span aria-hidden="true">←</span>;
}
