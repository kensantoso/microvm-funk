# microvm shells from a cmux button

Right-click `+` in cmux → fresh AWS Lambda MicroVM, SSH'd in, ~30 seconds.

Built on top of [@aidansteele](https://github.com/aidansteele)'s [microvmssh](../microvmssh) — that's the part that turns AWS's IAM-authed shell-ingress WebSocket into normal SSH. Everything in here just stands the infra up and wires a cmux action to launch one.

## what's in here

- `template.yml` — CloudFormation: S3 bucket, IAM roles, MicroVM image
- `deploy.sh` — zips the Dockerfile, uploads, deploys the stack
- `Dockerfile` — AL2023 + git, node, AWS CLI, Claude Code, the usual dev tools
- `launch-microvm.sh` — what the cmux action runs
- `cmux.json` — paste into `~/.config/cmux/cmux.json`
- `export-macos-gh-creds.sh` — push GitHub auth into a VM
- `export-macos-claude-creds.sh` — push Claude Code auth into a VM

## setup

```bash
./deploy.sh                                            # ~5 min
(cd ../microvmssh && go build -o ../bin/microvmssh .)
```

Add to `~/.ssh/config`:

```
Host *.microvm
    User root
    IdentitiesOnly yes
    StrictHostKeyChecking accept-new
    UserKnownHostsFile /dev/null
    ProxyCommand /full/path/to/microvm-funk/bin/microvmssh %h
```

You need any SSH key for the client to present (sshd accepts any; IAM does the real auth):

```bash
ssh-keygen -t ed25519 -f ~/.ssh/microvm_key -N "" -C microvm
```

Add `IdentityFile ~/.ssh/microvm_key` inside the `Host *.microvm` block.

Drop the contents of `cmux.json` into `~/.config/cmux/cmux.json`, update the command path to wherever you cloned this, then `cmux reload-config`.

## use it

Right-click cmux's titlebar `+` → **⚡ New disposable MicroVM**. A new workspace appears in the sidebar. ~30s later you're at a root prompt.

No creds loaded yet, by design. Push what you need from another tab:

```bash
./export-macos-gh-creds.sh <microvm-id>
./export-macos-claude-creds.sh <microvm-id>
```

The microvmId got saved to `~/.config/microvm/last` on launch — handy for scripting.

