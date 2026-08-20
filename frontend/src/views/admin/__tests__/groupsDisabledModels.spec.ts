import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

describe("group disabled models form", () => {
  it("binds, resets, and hydrates the policy in both group forms", () => {
    expect(groupsViewSource).toContain(
      'v-model:models="createForm.disabled_models"',
    );
    expect(groupsViewSource).toContain(
      'v-model:models="editForm.disabled_models"',
    );
    expect(groupsViewSource).toContain("createForm.disabled_models = [];");
    expect(groupsViewSource).toContain(
      "editForm.disabled_models = [...(group.disabled_models ?? [])];",
    );
  });
});
