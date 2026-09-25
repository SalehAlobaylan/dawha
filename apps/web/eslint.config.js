import eslint from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: ["dist", "node_modules", "test-results", "playwright-report"],
  },
  eslint.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["src/**/*.{ts,tsx}"],
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": ["warn", { allowConstantExport: true }],
    },
  },
  {
    // The Playwright config and the E2E suite run in Node, not in the browser,
    // so `process` is a real global there. Without this they would be linted
    // with the browser globals and every read of an environment variable would
    // be an error.
    files: ["playwright.config.ts", "e2e/**/*.ts", "e2e/**/*.mjs", "vite.config.ts", "tailwind.config.ts"],
    languageOptions: {
      globals: { process: "readonly", Buffer: "readonly", console: "readonly" },
    },
  },
);
