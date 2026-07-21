import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import prettierConfig from "eslint-config-prettier";

export default tseslint.config(
  // Base: ESLint recommended rules
  js.configs.recommended,

  // TypeScript: strict type checking (pakai tsconfig.json)
  ...tseslint.configs.recommended,

  // React Hooks: rules of hooks + exhaustive deps
  {
    plugins: { "react-hooks": reactHooks },
    rules: {
      ...reactHooks.configs.recommended.rules,
    },
  },

  // Prettier: matikan ESLint rules yg konflik dengan Prettier
  prettierConfig,

  // Global ignores
  {
    ignores: ["dist/", "node_modules/", "*.config.js"],
  },
);
