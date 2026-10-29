import { Link } from "@tanstack/react-router";
import { ArrowUpLeft, Bell, Search, SlidersHorizontal } from "lucide-react";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchSessionStatus } from "../lib/api";

interface TopBarProps {
  eyebrow: string;
  title: string;
  description: string;
}

export function TopBar({ eyebrow, title, description }: TopBarProps) {
  const [query, setQuery] = useState("");
  const session = useQuery({ queryKey: ["workspace-session"], queryFn: fetchSessionStatus, staleTime: 60000 });

  return (
    <header className="page-header">
      <div className="page-header-copy">
        <div className="eyebrow">{eyebrow}</div>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      <div className="page-header-actions">
        <form className="global-search" action="/search" method="get" role="search">
          <Search size={16} />
          <input
            name="q"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="ابحث في دَوْحة"
            aria-label="ابحث في دَوْحة"
          />
          <kbd>⌘ K</kbd>
        </form>
        <Link className="icon-button" to="/search" aria-label="تصفية">
          <SlidersHorizontal size={17} />
        </Link>
        <Link className="icon-button notification-button" to="/" hash="workspace-activity" aria-label="نشاط المساحة">
          <Bell size={17} />
        </Link>
        <Link to="/research" className="topbar-action" aria-label="افتح مكتب البحث">
          <ArrowUpLeft size={16} />
        </Link>
        <Link to="/login" className="avatar" aria-label={session.data ? "الحساب · متصل" : "تسجيل الدخول"}>م</Link>
      </div>
    </header>
  );
}
