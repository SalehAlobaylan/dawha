import { expect, test } from "@playwright/test";

import { API_URL, newAccount, registerThroughApi, registerThroughUi, waitForApi } from "../fixtures";

/**
 * Journey 1 of the declared V1 list: register.
 *
 * The journey is asserted at the boundary the reader actually touches - the
 * form - and then through the session endpoint, because the web app has no
 * signed-in banner to assert on and a form that redirects without a session
 * would look identical from the UI alone.
 */
test.describe("register", () => {
  test("a researcher can register and the API keeps a live session for them", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("register");

    await registerThroughUi(page, account);

    // The session cookie the form set is what every later journey rides on.
    const me = await page.request.get(`${API_URL}/api/v1/auth/me`);
    expect(me.status()).toBe(200);
    const body = (await me.json()) as { user?: { email?: string; display_name_ar?: string } };
    expect(body.user?.email).toBe(account.email);
    expect(body.user?.display_name_ar).toBe(account.displayName);

    // The account can sign in again from a clean browser, so the journey is not
    // only a one-shot redirect.
    await page.context().clearCookies();
    await page.goto("/login");
    await page.getByLabel("البريد الإلكتروني").fill(account.email);
    await page.getByLabel("كلمة المرور", { exact: true }).fill(account.password);
    await page.getByRole("button", { name: /دخول إلى دَوْحة/ }).click();
    await page.waitForURL((url) => !url.pathname.startsWith("/login"), { timeout: 20_000 });
    const again = await page.request.get(`${API_URL}/api/v1/auth/me`);
    expect(again.status()).toBe(200);
  });

  test("registration refuses a short password before it reaches the API", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("weak");

    await page.goto("/login");
    await page.getByRole("button", { name: "إنشاء حساب" }).click();
    await page.getByLabel("الاسم بالعربية").fill(account.displayName);
    await page.getByLabel("البريد الإلكتروني").fill(account.email);
    await page.getByLabel("كلمة المرور", { exact: true }).fill("قصير");
    await page.getByRole("button", { name: /إنشاء الحساب/ }).click();

    // The form's own minimum-length rule stops the submission, so the reader is
    // still on the form and no account exists. Asserting the absence of the
    // navigation is what makes this a refusal rather than a slow success.
    await page.waitForTimeout(1_000);
    expect(new URL(page.url()).pathname).toBe("/login");
    const created = await request.post(`${API_URL}/api/v1/auth/register`, {
      data: { email: account.email, password: "قصير", display_name_ar: account.displayName },
    });
    expect(created.status(), "the API must refuse the same password").toBe(400);
  });

  test("the register endpoint rejects a second account with the same address", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("duplicate");
    await registerThroughApi(request, account);

    await page.goto("/login");
    await page.getByRole("button", { name: "إنشاء حساب" }).click();
    await page.getByLabel("الاسم بالعربية").fill("باحث آخر");
    await page.getByLabel("البريد الإلكتروني").fill(account.email);
    await page.getByLabel("كلمة المرور", { exact: true }).fill(account.password);
    await page.getByRole("button", { name: /إنشاء الحساب/ }).click();

    // The alert names the refusal, so the reader is not left guessing why the
    // second attempt did not create anything.
    await expect(page.getByRole("alert")).toContainText("email is already registered");
    const me = await page.request.get(`${API_URL}/api/v1/auth/me`);
    expect(me.status(), "a refused registration must not leave a session").not.toBe(200);
  });
});
