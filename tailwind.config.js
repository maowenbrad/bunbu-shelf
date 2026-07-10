/** @type {import('tailwindcss').Config} */
// NOTE: the Tailwind v4 CLI reads the @theme block in web/static/input.css;
// this file only mirrors that palette for editor tooling. Keep them in sync.
module.exports = {
  content: ["./web/templates/**/*.html", "./internal/server/render.go"],
  theme: {
    extend: {
      fontFamily: {
        serif: ["Lora", "Georgia", "serif"],
        sans: ["Open Sans", "Segoe UI", "system-ui", "sans-serif"],
      },
      colors: {
        ink:         "#03151E",
        deep:        "#004557",
        teal:        "#0096B5",
        "teal-dark": "#007A93",
        cyan:        "#4BC1D2",
        ice:         "#F7FBFD",
        mist:        "#EAF3F4",
        line:        "#D5ECEF",
        fog:         "#5E8396",
        "fog-light": "#74A4B0",
        rose:        "#CD75A8",
        coral:       "#DF6C5E",
      },
    },
  },
  plugins: [],
};
