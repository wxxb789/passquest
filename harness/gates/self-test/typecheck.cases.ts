import { typecheck } from "../typecheck.ts";
import type { BrokenCase } from "./fixture.ts";

export function typecheckCases(): BrokenCase[] {
  return [
    {
      gate: typecheck,
      label: "TypeScript type error",
      files: {
        "tsconfig.json": JSON.stringify({
          compilerOptions: {
            target: "ES2022",
            module: "ESNext",
            moduleResolution: "bundler",
            strict: true,
            noEmit: true,
            skipLibCheck: true,
            types: [],
          },
          include: ["bad.ts"],
        }),
        "bad.ts": 'const value: number = "text";\n',
      },
      tracked: ["bad.ts", "tsconfig.json"],
    },
    {
      gate: typecheck,
      label: "type error in the desktop frontend project",
      files: {
        "tsconfig.json": JSON.stringify({
          compilerOptions: { target: "ES2022", module: "ESNext", strict: true, noEmit: true, types: [] },
          include: ["ok.ts"],
        }),
        "ok.ts": "export const ok: number = 1;\n",
        "desktop/web/tsconfig.json": JSON.stringify({
          compilerOptions: {
            target: "ES2022",
            module: "ESNext",
            moduleResolution: "bundler",
            lib: ["ES2022", "DOM"],
            strict: true,
            noEmit: true,
            types: [],
          },
          include: ["src/**/*.ts"],
        }),
        "desktop/web/src/main.ts": "export const length: number = document.body.innerText;\n",
      },
      tracked: ["ok.ts", "tsconfig.json", "desktop/web/tsconfig.json", "desktop/web/src/main.ts"],
      postFail: (failures) => {
        if (!failures.includes("desktop/web/tsconfig.json:")) return "typecheck: did not name the frontend project";
        if (failures.includes("tsconfig.json:")) return "typecheck: blamed the valid root project";
        return null;
      },
    },
    {
      gate: typecheck,
      label: "desktop frontend without its own project",
      files: { "desktop/web/src/main.ts": "export const ok = 1;\n" },
      tracked: ["desktop/web/src/main.ts"],
      postFail: (failures) =>
        failures.some((line) => line.includes("desktop/web/tsconfig.json is missing"))
          ? null
          : "typecheck: did not report the missing frontend project",
    },
  ];
}
