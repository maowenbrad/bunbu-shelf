/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./web/templates/**/*.html"],
  darkMode: "class",
  theme: {
    extend: {
      fontFamily: {
        serif: ["Lora", "Georgia", "serif"],
        sans: ["Inter", "system-ui", "sans-serif"],
      },
      colors: {
        netflix: {
          black:       "#141414",
          red:         "#E50914",
          "red-dark":  "#B20710",
          "red-hover": "#F40612",
          surface:     "#1f1f1f",
          "surface-2": "#2a2a2a",
          muted:       "#808080",
          border:      "#333333",
        },
      },
    },
  },
  plugins: [],
};
