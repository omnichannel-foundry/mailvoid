/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/web/templates/**/*.html",
    "./internal/web/static/app.js",
  ],
  safelist: [
    "badge-error",
    "badge-warning",
    "badge-ghost",
    "badge-info",
    "badge-success",
    "is-success",
    "is-warning",
    "is-accent",
  ],
  theme: {
    extend: {},
  },
  plugins: [require("daisyui")],
  daisyui: {
    themes: ["dark", "light"],
    darkTheme: "dark",
    base: true,
    styled: true,
    utils: true,
    logs: false,
  },
};
