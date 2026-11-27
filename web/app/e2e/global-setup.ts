import { request } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

const API_URL = process.env.E2E_API_URL || "http://localhost:8000";
const WEB_URL = process.env.E2E_BASE_URL || "http://localhost:3001";
const USERNAME =
  process.env.E2E_SHARED_USERNAME ||
  `e2e-shared-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
const PASSWORD = process.env.E2E_SHARED_PASSWORD || "e2e-password-123";
const STATE_PATH = path.join(process.cwd(), "e2e", ".auth", "shared.json");

async function ensureSharedUser() {
  const context = await request.newContext({ baseURL: API_URL });
  try {
    let loginResponse = await context.post("/v1/auth/login", {
      data: { username: USERNAME, password: PASSWORD },
    });
    if (loginResponse.status() === 401) {
      const registerResponse = await context.post("/v1/auth/register", {
        data: { username: USERNAME, password: PASSWORD },
      });
      if (!registerResponse.ok()) {
        throw new Error(
          `shared user registration failed: ${registerResponse.status()} ${await registerResponse.text()}`
        );
      }
      loginResponse = await context.post("/v1/auth/login", {
        data: { username: USERNAME, password: PASSWORD },
      });
    }
    if (!loginResponse.ok()) {
      throw new Error(
        `shared user login failed: ${loginResponse.status()} ${await loginResponse.text()}`
      );
    }
    return (await loginResponse.json()) as {
      userId: string;
      token: string;
      refreshToken: string;
    };
  } finally {
    await context.dispose();
  }
}

export default async function globalSetup() {
  const reply = await ensureSharedUser();
  await mkdir(path.dirname(STATE_PATH), { recursive: true });
  await writeFile(
    STATE_PATH,
    JSON.stringify(
      {
        cookies: [],
        origins: [
          {
            origin: WEB_URL,
            localStorage: [
              { name: "token", value: reply.token },
              { name: "refreshToken", value: reply.refreshToken },
              {
                name: "user",
                value: JSON.stringify({
                  userId: Number(reply.userId),
                  username: USERNAME,
                }),
              },
            ],
          },
        ],
      },
      null,
      2
    )
  );
}
