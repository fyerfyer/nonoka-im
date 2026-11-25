# Bifrost configuration

`config.json` is the demo/development config. Provider keys are read from env
(`DEEPSEEK_API_KEY`) and never stored by Nonoka.

## Production checklist

1. Pin the image (`maximhq/bifrost:v2.4.0` in `docker-compose.yml`).
2. Bind Bifrost to a private interface. The demo compose already publishes
   `127.0.0.1:8088`; do not expose 8080 publicly.
3. Enable dashboard authentication **and** keep inference virtual-key
   enforcement on. A secure config adds:

   ```json
   {
     "auth_config": {
       "is_enabled": true,
       "admin_username": "env.BIFROST_ADMIN_USERNAME",
       "admin_password": "env.BIFROST_ADMIN_PASSWORD"
     },
     "client": {
       "enforce_auth_on_inference": true,
       "disable_content_logging": true
     }
   }
   ```

4. Use `BIFROST_SETUP_TOKEN` only for the first admin bootstrap. After the
   admin account exists, run `scripts/bifrost-enable-auth.sh`, remove it from
   Nonoka/agent-worker, and supply `BIFROST_ADMIN_USERNAME` +
   `BIFROST_ADMIN_PASSWORD` (or `BIFROST_ADMIN_TOKEN`).
5. Keep `disable_content_logging` on unless the operator explicitly wants
   prompts stored in Bifrost logs. Nonoka's audit log never includes prompts.
6. Run the contract test with `NONOKA_BIFROST_LIVE=1` after upgrading Bifrost.

Nonoka creates one virtual key per Agent and one shared playground key. Both
are attached to the deployment customer budget; the global budget is the last
line of defense, while per-user/per-agent daily quotas are enforced by Nonoka.
