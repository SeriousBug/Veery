import { defineConfig } from "@pandacss/dev";

export default defineConfig({
  preflight: true,
  include: ["./src/**/*.{js,jsx,ts,tsx}"],
  exclude: [],
  jsxFramework: "react",
  outdir: "styled-system",

  theme: {
    extend: {
      tokens: {
        colors: {
          // Cheerful brand palette
          grape: {
            50: { value: "#f5f0ff" },
            100: { value: "#e9ddff" },
            200: { value: "#d3bbff" },
            300: { value: "#b892ff" },
            400: { value: "#9a63f7" },
            500: { value: "#7c3aed" },
            600: { value: "#6926d1" },
            700: { value: "#571ba8" },
            800: { value: "#421584" },
            900: { value: "#2f0f61" },
          },
          teal: {
            300: { value: "#5eead4" },
            400: { value: "#2dd4bf" },
            500: { value: "#14b8a6" },
            600: { value: "#0d9488" },
          },
          coral: {
            100: { value: "#ffe4e6" },
            200: { value: "#fecdd3" },
            300: { value: "#fda4af" },
            400: { value: "#fb7185" },
            500: { value: "#f43f5e" },
            600: { value: "#e11d48" },
          },
          sunshine: {
            300: { value: "#fde68a" },
            400: { value: "#fbbf24" },
            500: { value: "#f59e0b" },
          },
          mint: {
            300: { value: "#86efac" },
            400: { value: "#4ade80" },
            500: { value: "#22c55e" },
          },
          ink: {
            50: { value: "#f8f7fb" },
            100: { value: "#efedf5" },
            200: { value: "#dcd8e8" },
            400: { value: "#8b849e" },
            600: { value: "#4b455c" },
            800: { value: "#241f30" },
            900: { value: "#171320" },
          },
        },
        fonts: {
          body: {
            value:
              "'Nunito', 'Segoe UI', system-ui, -apple-system, sans-serif",
          },
          heading: {
            value:
              "'Nunito', 'Segoe UI', system-ui, -apple-system, sans-serif",
          },
        },
        radii: {
          sm: { value: "8px" },
          md: { value: "14px" },
          lg: { value: "20px" },
          xl: { value: "28px" },
          full: { value: "9999px" },
        },
        shadows: {
          card: { value: "0 6px 24px -8px rgba(124, 58, 237, 0.25)" },
          pop: { value: "0 12px 40px -12px rgba(124, 58, 237, 0.4)" },
        },
      },
      semanticTokens: {
        colors: {
          accent: { value: "{colors.grape.500}" },
          accentHover: { value: "{colors.grape.600}" },
          bg: { value: "{colors.ink.50}" },
          surface: { value: "white" },
          border: { value: "{colors.ink.200}" },
          text: { value: "{colors.ink.900}" },
          textMuted: { value: "{colors.ink.400}" },
          attention: { value: "{colors.coral.500}" },
          ok: { value: "{colors.mint.500}" },
        },
      },
    },
  },

  globalCss: {
    body: {
      fontFamily: "body",
      bg: "bg",
      color: "text",
    },
  },
});
