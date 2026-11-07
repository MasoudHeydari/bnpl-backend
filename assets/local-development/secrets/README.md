# Grafana admin password secret

Create a single-line password file (no trailing documentation text):

`make up` creates this file with a local default password when it is missing.
To set your own password before first start:

```bash
printf '%s' 'choose-a-strong-password' > assets/local-development/secrets/bnpl_loki_grafana_password
```

`make up` sets mode `644` so the Grafana process (non-root UID 472) can read the
file-backed Docker secret. Do not chmod it to `600`; Compose on this engine
inherits host permissions and Grafana cannot read an owner-only secret.

Never commit the real password file (gitignored).
