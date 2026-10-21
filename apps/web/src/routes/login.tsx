import { Link } from "@tanstack/react-router";
import { ArrowLeft, ArrowRight, Eye, EyeOff, LockKeyhole, Mail, UserRound } from "lucide-react";
import { FormEvent, useState } from "react";
import { BrandMark } from "../components/BrandMark";
import { getSafeReturnPath } from "../lib/return-path";

const apiBaseUrl = import.meta.env.VITE_API_URL ?? "";

export function LoginPage() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [quickSignInBusy, setQuickSignInBusy] = useState(false);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    if (!apiBaseUrl) {
      setMessage("شغّل خدمة Core API أولاً لتفعيل الحساب.");
      return;
    }
    setBusy(true);
    try {
      const endpoint = mode === "login" ? "/api/v1/auth/login" : "/api/v1/auth/register";
      const body = mode === "login" ? { email, password } : { email, password, display_name_ar: displayName };
      const response = await fetch(`${apiBaseUrl}${endpoint}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(body),
      });
      const result = (await response.json()) as { error?: string };
      if (!response.ok) {
        setMessage(result.error ?? "تعذر إكمال العملية.");
        return;
      }
      window.location.assign(getSafeReturnPath(new URLSearchParams(window.location.search).get("returnTo"), window.location.origin));
    } catch {
      setMessage("تعذر الاتصال بالخدمة. حاول مرة أخرى.");
    } finally {
      setBusy(false);
    }
  };

  const quickSignIn = async () => {
    setMessage("");
    if (!apiBaseUrl) {
      setMessage("شغّل خدمة Core API أولاً لتفعيل الدخول المحلي.");
      return;
    }
    setQuickSignInBusy(true);
    try {
      const response = await fetch(`${apiBaseUrl}/api/v1/auth/dev/quick-sign-in`, {
        method: "POST",
        credentials: "include",
      });
      const result = (await response.json()) as { error?: string };
      if (!response.ok) {
        setMessage(result.error ?? "الدخول المحلي غير مفعّل في خدمة Core API.");
        return;
      }
      window.location.assign(getSafeReturnPath(new URLSearchParams(window.location.search).get("returnTo"), window.location.origin));
    } catch {
      setMessage("تعذر الاتصال بخدمة Core API للدخول المحلي.");
    } finally {
      setQuickSignInBusy(false);
    }
  };

  return (
    <div className="auth-page">
      <div className="auth-orbit orbit-left" />
      <div className="auth-orbit orbit-right" />
      <div className="auth-topline"><Link to="/" className="auth-brand"><BrandMark /><strong>دَوْحة</strong></Link><Link to="/" className="auth-back"><ArrowRight size={15} /> العودة إلى المساحة</Link></div>
      <main className="auth-card">
        <div className="auth-card-intro"><div className="eyebrow">حساب الباحث</div><h1>{mode === "login" ? "مرحباً بعودتك" : "ابدأ سياقك"}</h1><p>{mode === "login" ? "ادخل إلى مساحة العمل حيث تبقى المصادر والخلافات قابلة للتتبع." : "أنشئ حساباً لحفظ أبحاثك، والعودة إلى الأسئلة التي لم تُحسم."}</p></div>
        <div className="auth-mode-tabs"><button type="button" className={mode === "login" ? "auth-mode-active" : ""} onClick={() => setMode("login")}>تسجيل الدخول</button><button type="button" className={mode === "register" ? "auth-mode-active" : ""} onClick={() => setMode("register")}>إنشاء حساب</button></div>
        <form className="auth-form" onSubmit={submit}>
          {mode === "register" ? <label className="auth-field"><span>الاسم بالعربية</span><div><UserRound size={16} /><input required value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="مثال: باحث نجم" /></div></label> : null}
          <label className="auth-field"><span>البريد الإلكتروني</span><div><Mail size={16} /><input required type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@example.com" /></div></label>
          <label className="auth-field"><span>كلمة المرور</span><div><LockKeyhole size={16} /><input required minLength={12} type={showPassword ? "text" : "password"} value={password} onChange={(event) => setPassword(event.target.value)} placeholder="12 حرفاً على الأقل" /><button type="button" onClick={() => setShowPassword((visible) => !visible)} aria-label={showPassword ? "إخفاء كلمة المرور" : "إظهار كلمة المرور"}>{showPassword ? <EyeOff size={16} /> : <Eye size={16} />}</button></div></label>
          {message ? <div className="auth-message" role="alert">{message}</div> : null}
          <button className="primary-button auth-submit" type="submit" disabled={busy}>{busy ? "جارٍ التحقق…" : mode === "login" ? "دخول إلى دَوْحة" : "إنشاء الحساب"}<ArrowLeft size={16} /></button>
        </form>
        {import.meta.env.DEV && mode === "login" ? <div className="auth-dev-access">
          <div className="auth-dev-divider"><span>للتجربة المحلية</span></div>
          <button className="auth-dev-sign-in" type="button" onClick={quickSignIn} disabled={quickSignInBusy || busy}>
            {quickSignInBusy ? "جارٍ فتح الحساب المحلي…" : "دخول سريع بحساب المراجع المحلي"}
          </button>
          <small>حساب مؤقت في بيئة التطوير فقط؛ لا تستخدمه على موقع منشور.</small>
        </div> : null}
        <div className="auth-note"><span className="auth-note-mark">؟</span><span>لا نستخدم DNA. الحساب يساعدنا على حفظ أبحاثك، لا على إعلان حقيقتك.</span></div>
      </main>
      <div className="auth-bottom"><span>بيئة بحث عربية</span><span>المصدر أولاً · الخلاف محفوظ</span></div>
    </div>
  );
}
