# Kronos systemd Units

These units are production-oriented starting points for bare-metal or VM
deployments. Install the Kronos binary at `/usr/local/bin/kronos`, place the
control-plane config at `/etc/kronos/kronos.yaml`, and keep secrets in root-only
environment files or a managed secret injector.

```bash
sudo install -o root -g root -m 0755 ./bin/kronos /usr/local/bin/kronos
sudo install -o root -g root -m 0644 contrib/systemd/kronos-server.service /etc/systemd/system/kronos-server.service
sudo install -o root -g root -m 0644 contrib/systemd/kronos-agent.service /etc/systemd/system/kronos-agent.service
sudo install -o root -g root -m 0640 contrib/systemd/kronos-server.env.example /etc/kronos/kronos-server.env
sudo install -o root -g root -m 0640 contrib/systemd/kronos-agent.env.example /etc/kronos/kronos-agent.env
sudo systemctl daemon-reload
sudo systemctl enable --now kronos-server
sudo systemctl enable --now kronos-agent
```

Before enabling the services:

- Replace every placeholder in `/etc/kronos/kronos-server.env` and
  `/etc/kronos/kronos-agent.env`.
- Create a dedicated `kronos` user and group or adjust the `User=`/`Group=`
  directives to match your host policy.
- Create `/etc/kronos/kronos.yaml` with production auth enabled and a stable
  `server.data_dir`, preferably `/var/lib/kronos`.
- Store TLS material under `/etc/kronos/tls` when direct TLS or mTLS is used.
- Grant the agent host access to required database client tools and repository
  credentials.

The server unit uses `StateDirectory=kronos`, so systemd creates
`/var/lib/kronos` for the embedded state database on systems that support the
directive. Keep that directory on durable storage and include it in host backup
and rollback procedures.
