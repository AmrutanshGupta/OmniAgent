import crypto from "crypto";

const ALGO = "aes-256-gcm";

function getKey() {
  const keyHex = process.env.ENCRYPTION_KEY;
  if (!keyHex) {
    throw new Error("ENCRYPTION_KEY is not set.");
  }
  const key = Buffer.from(keyHex, "hex");
  return key;
}

export function encrypt(plaintext) {
  if (!plaintext) return "";
  const key = getKey();
  const iv = crypto.randomBytes(12);
  const cipher = crypto.createCipheriv(ALGO, key, iv);
  const encrypted = Buffer.concat([cipher.update(String(plaintext), "utf8"), cipher.final()]);
  const authTag = cipher.getAuthTag();
  return Buffer.concat([iv, authTag, encrypted]).toString("base64");
}

export function decrypt(payload) {
  if (!payload) return "";
  const key = getKey();
  const raw = Buffer.from(payload, "base64");
  const iv = raw.subarray(0, 12);
  const authTag = raw.subarray(12, 28);
  const ciphertext = raw.subarray(28);
  const decipher = crypto.createDecipheriv(ALGO, key, iv);
  decipher.setAuthTag(authTag);
  const decrypted = Buffer.concat([decipher.update(ciphertext), decipher.final()]);
  return decrypted.toString("utf8");
}

export function encryptKeys(keys = {}) {
  return {
    openai: encrypt(keys.openai ?? ""),
    anthropic: encrypt(keys.anthropic ?? ""),
    google: encrypt(keys.google ?? ""),
    groq: encrypt(keys.groq ?? ""),
    huggingface: encrypt(keys.huggingface ?? ""),
  };
}

export function decryptKeys(keys = {}) {
  const safeDecrypt = (v) => {
    try { return decrypt(v); } catch { return ""; }
  };
  return {
    openai: safeDecrypt(keys.openai),
    anthropic: safeDecrypt(keys.anthropic),
    google: safeDecrypt(keys.google),
    groq: safeDecrypt(keys.groq),
    huggingface: safeDecrypt(keys.huggingface),
  };
}