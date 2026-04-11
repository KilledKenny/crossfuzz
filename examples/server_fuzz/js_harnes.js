import { run } from "../../harness/js/crossfuzz";
const url = "http://127.0.0.1:8000";

function fuzzInflate(bytes) {
  const out = [];

  for (let i = 0; i < bytes.length; i++) {
    const b = bytes[i];

    // "fuzz inflate" rules:
    // - ignore null bytes
    // - keep printable ASCII
    // - replace others with space
    if (b === 0x00) continue;

    if (b >= 32 && b <= 126) {
      out.push(String.fromCharCode(b));
    } else {
      out.push(" ");
    }
  }

  return out.join("").replace(/\s+/g, " ").trim();
}

function extractHeadersFromText(text) {
  const headers = {};

  // Very simple heuristic parsing:
  // key:value pairs separated by newlines or semicolons
  const parts = text.split(/[\n;]/);

  for (const part of parts) {
    const idx = part.indexOf(":");
    if (idx === -1) continue;

    const key = part.slice(0, idx).trim();
    const value = part.slice(idx + 1).trim();

    if (key && value) {
      headers[key] = value;
    }
  }

  return headers;
}

export function fuzzToFetchHeaders(uint8) {
  const text = fuzzInflate(uint8);

  const parsed = extractHeadersFromText(text);

  // Always ensure valid fetch headers fallback
  return {
    "user-agent": parsed["User-Agent"] || "bun-fuzzer/1.0",
    "content-type": parsed["Content-Type"] || "application/octet-stream",
    "x-fuzzed": "true",
    ...parsed,
  };
}

async function harnes(input) {
  const headers = fuzzToFetchHeaders(input);
  try {
    const response = await fetch(url + `/?data=${input}`, { headers: headers });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    // Read response as plain text (NOT JSON)
    const text = await response.text();

    //console.log("Response from server:");
    //console.log(text);
  } catch (error) {
    return await harnes("")
    //console.error("Request failed:", error.message);
  }

}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function main() {
  //Sleep 1 sec to give the servers a chance to start before we connect to the fuzzer
  await sleep(1000);

  // This harness only triggers behavior in the server targets; its own
  // coverage is not meaningful.
  run(harnes, { disableInstrumentation: true });
}

main();
