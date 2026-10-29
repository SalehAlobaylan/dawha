import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  BookOpen,
  Compass,
  GitBranch,
  LayoutDashboard,
  LockKeyhole,
  Map,
  Menu,
  Plus,
  Search,
  Sparkles,
  X,
} from "lucide-react";
import { useState } from "react";
import { BrandMark } from "./BrandMark";
import { fetchSessionStatus } from "../lib/api";

const workspaceNavigation = [
  { to: "/", label: "الرئيسية", icon: LayoutDashboard },
  { to: "/tree", label: "أشجاري", icon: GitBranch },
  { to: "/places", label: "الخريطة الجغرافية", icon: Map },
];

const discoveryNavigation = [
  { to: "/explore", label: "تصفح الأشجار", icon: Compass },
  { to: "/sources", label: "المصادر", icon: BookOpen },
];

export function AppShell() {
  const [mobileOpen, setMobileOpen] = useState(false);
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const fromExplore = useRouterState({ select: (state) => (state.location.search as { from?: string }).from === "explore" && state.location.pathname.startsWith("/tree/") });
  const session = useQuery({ queryKey: ["workspace-session"], queryFn: fetchSessionStatus, staleTime: 60000, enabled: pathname !== "/login" && !pathname.startsWith("/invitation/") });

  if (pathname === "/login" || pathname.startsWith("/invitation/")) {
    return <Outlet />;
  }

  const navLink = ({ to, label, icon: Icon }: typeof workspaceNavigation[number]) => {
    const active = to === "/" ? pathname === "/" : to === "/tree" ? !fromExplore && (pathname.startsWith("/tree") || pathname.startsWith("/names") || pathname === "/import") : to === "/explore" ? pathname === to || fromExplore : pathname === to || pathname.startsWith(`${to}/`);
    return <Link key={to} to={to} activeOptions={{ exact: true }} aria-current={active ? pathname === to ? "page" : "location" : undefined} className={`nav-link${active ? " nav-link-active" : ""}`} onClick={() => setMobileOpen(false)}><Icon size={18} strokeWidth={active ? 2.2 : 1.7} /><span>{label}</span></Link>;
  };

  return (
    <div className="app-shell">
      <button
        className="mobile-menu-button"
        type="button"
        aria-expanded={mobileOpen}
        aria-controls="workspace-navigation"
        aria-label={mobileOpen ? "إغلاق القائمة" : "فتح القائمة"}
        onClick={() => setMobileOpen((open) => !open)}
      >
        {mobileOpen ? <X size={19} /> : <Menu size={19} />}
      </button>
      {mobileOpen ? <button className="mobile-nav-backdrop" type="button" aria-label="إغلاق القائمة الجانبية" onClick={() => setMobileOpen(false)} /> : null}
      <aside id="workspace-navigation" className={`sidebar${mobileOpen ? " sidebar-open" : ""}`}>
        <div className="sidebar-top">
          <Link to="/" className="brand" onClick={() => setMobileOpen(false)}>
            <BrandMark />
            <span>
              <strong>دَوْحة</strong>
              <small>بيئة بحث النسب</small>
            </span>
          </Link>
          <span className="workspace-chip">مساحة نجم</span>
        </div>

        <section className="sidebar-group sidebar-workspace" aria-labelledby="workspace-nav-heading">
          <h2 id="workspace-nav-heading" className="sidebar-section-label">مساحتك</h2>
          <nav className="primary-nav" aria-label="التنقل الرئيسي">
            {workspaceNavigation.map(navLink)}
          </nav>
        </section>

        <section className="sidebar-group sidebar-discovery" aria-labelledby="discovery-nav-heading">
          <h2 id="discovery-nav-heading" className="sidebar-section-label">اكتشف</h2>
          <nav className="secondary-nav" aria-label="اكتشف">{discoveryNavigation.map(navLink)}</nav>
        </section>

        <section className="sidebar-group sidebar-research" aria-labelledby="research-nav-heading">
          <h2 id="research-nav-heading" className="sidebar-section-label">البحث</h2>
          <nav className="secondary-nav" aria-label="البحث">{navLink({ to: "/research", label: "مكتب البحث", icon: Sparkles })}{navLink({ to: "/search", label: "البحث والفهرس", icon: Search })}</nav>
        </section>

        <div className="sidebar-bottom">
          <div className="layer-note">
            <div className="layer-note-icon"><Sparkles size={15} /></div>
            <div><strong>ابدأ من سؤال</strong><span>اترك تفسير الشجرة مفتوحاً.</span></div>
          </div>
          <Link to="/research" search={{ create: true }} className="sidebar-research-link" onClick={() => setMobileOpen(false)}>
            <Plus size={16} /><span>بحث جديد</span>
          </Link>
          <Link className="sidebar-settings" to="/login" onClick={() => setMobileOpen(false)}><LockKeyhole size={16} /><span>{session.data ? "الحساب · متصل" : "تسجيل الدخول"}</span></Link>
        </div>
      </aside>
      <main className="main-shell">
        <div className="mobile-topbar">
          <Link to="/" className="mobile-brand" onClick={() => setMobileOpen(false)}>
            <BrandMark />
            <strong>دَوْحة</strong>
          </Link>
          <Link to="/login" className="avatar avatar-small" aria-label={session.data ? "الحساب · متصل" : "تسجيل الدخول"}>م</Link>
        </div>
        <Outlet />
      </main>
    </div>
  );
}
