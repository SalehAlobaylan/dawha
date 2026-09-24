import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import {
  BookOpen,
  ChevronLeft,
  CircleHelp,
  Compass,
  GitBranch,
  LayoutDashboard,
  Map,
  Menu,
  Plus,
  Search,
  Settings2,
  Sparkles,
  X,
} from "lucide-react";
import { useState } from "react";
import { BrandMark } from "./BrandMark";

const navigation = [
  { to: "/", label: "نظرة عامة", icon: LayoutDashboard },
  { to: "/tree", label: "شجرة بحث", icon: GitBranch },
  { to: "/research", label: "مكتب البحث", icon: Sparkles },
  { to: "/sources", label: "المصادر", icon: BookOpen },
  { to: "/places", label: "المواضع", icon: Map },
  { to: "/questions", label: "الأسئلة", icon: CircleHelp },
];

export function AppShell() {
  const [mobileOpen, setMobileOpen] = useState(false);
  const pathname = useRouterState({ select: (state) => state.location.pathname });

  if (pathname === "/login" || pathname.startsWith("/invitation/")) {
    return <Outlet />;
  }

  return (
    <div className="app-shell">
      <button
        className="mobile-menu-button"
        type="button"
        aria-label={mobileOpen ? "إغلاق القائمة" : "فتح القائمة"}
        onClick={() => setMobileOpen((open) => !open)}
      >
        {mobileOpen ? <X size={19} /> : <Menu size={19} />}
      </button>
      <aside className={`sidebar${mobileOpen ? " sidebar-open" : ""}`}>
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

        <div className="sidebar-section-label">مساحة العمل</div>
        <nav className="primary-nav" aria-label="التنقل الرئيسي">
          {navigation.map(({ to, label, icon: Icon }) => {
            const active = to === "/" ? pathname === "/" : pathname === to || pathname.startsWith(`${to}/`);
            return (
              <Link
                key={to}
                to={to}
                className={`nav-link${active ? " nav-link-active" : ""}`}
                activeProps={{ "aria-current": "page" }}
                onClick={() => setMobileOpen(false)}
              >
                <Icon size={17} strokeWidth={active ? 2.2 : 1.7} />
                <span>{label}</span>
                {label === "الأسئلة" ? <span className="nav-count">89</span> : null}
              </Link>
            );
          })}
        </nav>

        <div className="sidebar-divider" />
        <div className="sidebar-section-label">اكتشف</div>
        <nav className="secondary-nav" aria-label="روابط الاستكشاف">
          <a className="nav-link" href="#dictionary" onClick={() => setMobileOpen(false)}>
            <Compass size={17} strokeWidth={1.7} />
            <span>فهرس العائلات</span>
            <ChevronLeft className="nav-chevron" size={15} />
          </a>
          <a className="nav-link" href="#saved" onClick={() => setMobileOpen(false)}>
            <Search size={17} strokeWidth={1.7} />
            <span>المحفوظات</span>
            <ChevronLeft className="nav-chevron" size={15} />
          </a>
        </nav>

        <div className="sidebar-bottom">
          <div className="layer-note">
            <div className="layer-note-icon"><Sparkles size={15} /></div>
            <div>
              <strong>ابدأ من سؤال</strong>
              <span>اترك تفسير الشجرة مفتوحاً.</span>
            </div>
          </div>
          <Link to="/research" className="new-research-button" onClick={() => setMobileOpen(false)}>
            <Plus size={16} />
            <span>بحث جديد</span>
          </Link>
          <button className="sidebar-settings" type="button">
            <Settings2 size={16} />
            <span>إعدادات المساحة</span>
          </button>
        </div>
      </aside>
      <main className="main-shell">
        <div className="mobile-topbar">
          <Link to="/" className="mobile-brand" onClick={() => setMobileOpen(false)}>
            <BrandMark />
            <strong>دَوْحة</strong>
          </Link>
          <Link to="/login" className="avatar avatar-small" aria-label="تسجيل الدخول">م</Link>
        </div>
        <Outlet />
      </main>
    </div>
  );
}
