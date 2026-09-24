import { Link } from "@tanstack/react-router";
import { ArrowUpLeft, Bell, Search, SlidersHorizontal } from "lucide-react";
import { useState } from "react";

interface TopBarProps {
  eyebrow: string;
  title: string;
  description: string;
}

export function TopBar({ eyebrow, title, description }: TopBarProps) {
  const [query, setQuery] = useState("");

  return (
    <header className="page-header">
      <div className="page-header-copy">
        <div className="eyebrow">{eyebrow}</div>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      <div className="page-header-actions">
        <label className="global-search">
          <Search size={16} />
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="ابحث في دَوْحة"
            aria-label="ابحث في دَوْحة"
          />
          <kbd>⌘ K</kbd>
        </label>
        <button className="icon-button" type="button" aria-label="تصفية">
          <SlidersHorizontal size={17} />
        </button>
        <button className="icon-button notification-button" type="button" aria-label="الإشعارات">
          <Bell size={17} />
          <span className="notification-dot" />
        </button>
        <Link to="/research" className="topbar-action" aria-label="افتح مكتب البحث">
          <ArrowUpLeft size={16} />
        </Link>
        <Link to="/login" className="avatar" aria-label="تسجيل الدخول">م</Link>
      </div>
    </header>
  );
}
