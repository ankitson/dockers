#!/usr/bin/env node

import { readFile, writeFile } from "node:fs/promises";

const target = "/usr/lib/node_modules/openclaw/dist/registry-BmAQAf7L.js";

const before = `\tconst body = {\n\t\tmodel: client.model,\n\t\tinput,\n\t\t...typeof client.dimensions === "number" ? { dimensions: client.dimensions } : {},\n\t\t...inputType ? { input_type: inputType } : {}\n\t};`;

const after = `\tconst passthroughExtraParams = client.headers["x-bf-passthrough-extra-params"] === "true";\n\tconst body = {\n\t\tmodel: client.model,\n\t\tinput,\n\t\t...typeof client.dimensions === "number" ? { dimensions: client.dimensions } : {},\n\t\t...inputType ? passthroughExtraParams ? { extra_params: { input_type: inputType } } : { input_type: inputType } : {}\n\t};`;

const source = await readFile(target, "utf8");
if (!source.includes(before)) {
  throw new Error(`openclaw bifrost embeddings patch: expected snippet not found in ${target}`);
}

const patched = source.replace(before, after);
if (patched === source) {
  throw new Error(`openclaw bifrost embeddings patch: replacement did not change ${target}`);
}

await writeFile(target, patched);
console.log(`patched ${target} for bifrost embedding passthrough`);
