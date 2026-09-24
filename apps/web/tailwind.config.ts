import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        canvas: "#E9DFC9",
        surface: "#F3EBDD",
        ink: "#2D2520",
        muted: "#6E6257",
        red: "#994438",
        clay: "#B66F4D",
        olive: "#747153",
        sand: "#C79D62",
        rule: "#CDBFA9",
      },
    },
  },
  plugins: [],
} satisfies Config;
