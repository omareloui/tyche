# AWS deployment

One `t4g.micro` ECS host (ECS-optimized AL2023, arm64) in `us-east-1a`, no load balancer:

- **Task** `tyche-task` (`networkMode: host`): `tyche-container` on `:3333` + `caddy` on `:80`/`:443` (`caddy reverse-proxy`, Let's Encrypt certs).
- **Data** on a separate encrypted gp3 volume `tyche-data`, mounted at `/srv/tyche` (`data/` → `/app/data`, `logs/` → `/app/logs`, `caddy/` → Caddy's certs). It outlives the instance (`DeleteOnTermination=false`).
- **Secrets** in SSM Parameter Store: `/tyche/JWT_SECRET`, `/tyche/GOOGLE_CLIENT_ID`, `/tyche/GOOGLE_CLIENT_SECRET` (SecureString), `/tyche/ADMIN_TENANT_IDS`.
- **Backups**: a DLM policy snapshots every volume tagged `Backup=tyche` daily at 03:00 UTC (keeps 14) and copies them to `eu-central-1` (kept 7 days). The tenant DBs and `shared.sqlite` (the vault keys of the encrypted tenants) share the volume, so they're always snapshotted together.
- **Access**: no SSH, use SSM (`aws ssm start-session --target <instance id>`).
- **Deploys**: pushing to `main` runs `.github/workflows/deploy.yml` (arm64 runner → ECR → new `tyche-task` revision). The service runs a single task (`minimumHealthyPercent=0`): the old task stops before the new one starts, a few seconds of downtime.

## Setup

```bash
ADMIN_TENANT_IDS=GL_123 aws/infra/up.sh [image]
```

`up.sh` creates what's missing (default VPC, security group, IAM roles, SSM params, data volume, instance, Elastic IP, ECR lifecycle, log retention, snapshot policy, task definition, service) and prints the public IP. It can be re-run. Then:

1. Point the A record of `tyche.omareloui.com` to the Elastic IP.
2. Add `https://tyche.omareloui.com/oauth/cb` (and the mobile callback) to the Google OAuth client's redirect URIs.

## Restore a snapshot

```bash
aws ecs update-service --cluster tyche --service tyche-service --desired-count 0
aws ec2 create-volume --snapshot-id <snap> --availability-zone us-east-1a --volume-type gp3 \
  --tag-specifications 'ResourceType=volume,Tags=[{Key=Name,Value=tyche-data-restored},{Key=Backup,Value=tyche}]'
aws ec2 detach-volume --volume-id <old volume>
aws ec2 attach-volume --volume-id <new volume> --instance-id <instance> --device /dev/sdf
aws ec2 reboot-instances --instance-ids <instance>  # remounts by label
aws ecs update-service --cluster tyche --service tyche-service --desired-count 1
```

Then tag the new volume `Name=tyche-data` (and the old one something else) so `up.sh` finds it. A snapshot of another region is copied back first (`aws ec2 copy-snapshot --source-region eu-central-1 ...`).

## Cost (on-demand, ≈ $13/mo)

t4g.micro $6.13 · Elastic IP $3.65 · root 30 GB $2.40 · data 5 GB $0.40 · snapshots ~$0.30 · ECR + logs ~$0.50.
