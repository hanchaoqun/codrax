import { ApiClient } from "@app/client";

/** CLI entry: `appctl <user-id>` prints the user record as JSON. */
export async function run(argv: string[]): Promise<number> {
  const id = argv[0];
  if (!id) {
    console.error("usage: appctl <user-id>");
    return 2;
  }
  const client = new ApiClient(process.env.APP_API_URL ?? "http://localhost:8080");
  try {
    const user = await client.fetchUser(id);
    console.log(JSON.stringify(user, null, 2));
    return 0;
  } catch (err) {
    console.error(`lookup failed: ${(err as Error).message}`);
    return 1;
  }
}

run(process.argv.slice(2)).then((code) => {
  process.exitCode = code;
});
