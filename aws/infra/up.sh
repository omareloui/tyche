#!/usr/bin/env bash
# Creates (or completes) the tyche infrastructure: one t4g ECS host with the
# DBs on a separate EBS volume, Caddy for TLS, daily snapshots. Safe to re-run:
# existing resources are looked up by name/tag and kept.
#
#   [ADMIN_TENANT_IDS=GL_123] [FROM_REGION=us-east-1] aws/infra/up.sh [image]
#
# With an image the task definition is registered with it and the service runs
# 1 task; without one the existing task definition is kept (the first run needs
# an image, CI then pushes the next ones).
set -euo pipefail

cd "$(dirname "$0")"

IMAGE="${1:-}"
REGION="${AWS_REGION:-eu-central-1}"
AZ="${AZ:-eu-central-1a}"
COPY_REGION="${COPY_REGION:-eu-west-1}"
# Region to copy the /tyche/* SSM params from when missing here (moving region).
FROM_REGION="${FROM_REGION:-}"
DOMAIN="${DOMAIN:-tychefortune.com}"
INSTANCE_TYPE="${INSTANCE_TYPE:-t4g.micro}"
DATA_SIZE_GB="${DATA_SIZE_GB:-5}"
CLUSTER=tyche
SERVICE=tyche-service
FAMILY=tyche-task

export AWS_REGION="$REGION" AWS_PAGER=""
ACCOUNT=$(aws sts get-caller-identity --query Account --output text)

log() { printf '\033[1m==> %s\033[0m\n' "$*"; }
none() { [ -z "$1" ] || [ "$1" = "None" ]; }

# --- Network ---------------------------------------------------------------
VPC_ID=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)
if none "$VPC_ID"; then
  log "Creating the default VPC"
  VPC_ID=$(aws ec2 create-default-vpc --query Vpc.VpcId --output text)
  aws ec2 wait vpc-available --vpc-ids "$VPC_ID"
fi
SUBNET_ID=$(aws ec2 describe-subnets \
  --filters Name=vpc-id,Values="$VPC_ID" Name=availability-zone,Values="$AZ" Name=default-for-az,Values=true \
  --query 'Subnets[0].SubnetId' --output text)
if none "$SUBNET_ID"; then
  log "Creating the default subnet of $AZ"
  SUBNET_ID=$(aws ec2 create-default-subnet --availability-zone "$AZ" --query Subnet.SubnetId --output text)
fi

SG_ID=$(aws ec2 describe-security-groups \
  --filters Name=vpc-id,Values="$VPC_ID" Name=group-name,Values=tyche-ec2-sg \
  --query 'SecurityGroups[0].GroupId' --output text)
if none "$SG_ID"; then
  log "Creating the security group"
  SG_ID=$(aws ec2 create-security-group --group-name tyche-ec2-sg \
    --description "tyche ECS host: HTTP(S) from anywhere" --vpc-id "$VPC_ID" \
    --query GroupId --output text)
  aws ec2 authorize-security-group-ingress --group-id "$SG_ID" --ip-permissions \
    'IpProtocol=tcp,FromPort=80,ToPort=80,IpRanges=[{CidrIp=0.0.0.0/0}],Ipv6Ranges=[{CidrIpv6=::/0}]' \
    'IpProtocol=tcp,FromPort=443,ToPort=443,IpRanges=[{CidrIp=0.0.0.0/0}],Ipv6Ranges=[{CidrIpv6=::/0}]' >/dev/null
fi

# --- Secrets (copied once from FROM_REGION when moving region) -----------
for name in JWT_SECRET GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET ADMIN_TENANT_IDS; do
  aws ssm get-parameter --name "/tyche/$name" >/dev/null 2>&1 && continue
  type=SecureString
  [ "$name" = ADMIN_TENANT_IDS ] && type=String
  if [ -n "$FROM_REGION" ]; then
    log "Copying $name from $FROM_REGION"
    value=$(aws ssm get-parameter --region "$FROM_REGION" --name "/tyche/$name" --with-decryption \
      --query Parameter.Value --output text)
  elif [ "$name" = ADMIN_TENANT_IDS ] && [ -n "${ADMIN_TENANT_IDS:-}" ]; then
    value="$ADMIN_TENANT_IDS"
  else
    echo "no /tyche/$name, put it with: aws ssm put-parameter --type $type --name /tyche/$name --value ..." >&2
    echo "(or set FROM_REGION to copy it, ADMIN_TENANT_IDS for the admins)" >&2
    exit 1
  fi
  aws ssm put-parameter --name "/tyche/$name" --type "$type" --value "$value" >/dev/null
  unset value
done

# --- Web Push keys (the app badge), generated once -----------------------
if ! aws ssm get-parameter --name /tyche/VAPID_PRIVATE_KEY >/dev/null 2>&1; then
  if [ -n "$FROM_REGION" ]; then
    log "Copying the VAPID keys from $FROM_REGION"
    VAPID_PUBLIC_KEY=$(aws ssm get-parameter --region "$FROM_REGION" --name /tyche/VAPID_PUBLIC_KEY \
      --query Parameter.Value --output text)
    VAPID_PRIVATE_KEY=$(aws ssm get-parameter --region "$FROM_REGION" --name /tyche/VAPID_PRIVATE_KEY --with-decryption \
      --query Parameter.Value --output text)
  else
    log "Generating the VAPID keys"
    eval "$(cd ../.. && go run ./cmd/vapid)"
  fi
  aws ssm put-parameter --name /tyche/VAPID_PUBLIC_KEY --type String --value "$VAPID_PUBLIC_KEY" --overwrite >/dev/null
  aws ssm put-parameter --name /tyche/VAPID_PRIVATE_KEY --type SecureString --value "$VAPID_PRIVATE_KEY" --overwrite >/dev/null
  unset VAPID_PUBLIC_KEY VAPID_PRIVATE_KEY
fi

# --- IAM -------------------------------------------------------------------
aws iam put-role-policy --role-name ecsTaskExecutionRole --policy-name tyche-ssm-secrets \
  --policy-document "{
    \"Version\": \"2012-10-17\",
    \"Statement\": [{
      \"Effect\": \"Allow\",
      \"Action\": \"ssm:GetParameters\",
      \"Resource\": \"arn:aws:ssm:$REGION:$ACCOUNT:parameter/tyche/*\"
    }]
  }"

if ! aws iam get-role --role-name tyche-ecs-instance >/dev/null 2>&1; then
  log "Creating the instance role"
  aws iam create-role --role-name tyche-ecs-instance --assume-role-policy-document '{
    "Version": "2012-10-17",
    "Statement": [{"Effect": "Allow", "Principal": {"Service": "ec2.amazonaws.com"}, "Action": "sts:AssumeRole"}]
  }' >/dev/null
  aws iam attach-role-policy --role-name tyche-ecs-instance \
    --policy-arn arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role
  aws iam attach-role-policy --role-name tyche-ecs-instance \
    --policy-arn arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore
fi
if ! aws iam get-instance-profile --instance-profile-name tyche-ecs-instance >/dev/null 2>&1; then
  aws iam create-instance-profile --instance-profile-name tyche-ecs-instance >/dev/null
  aws iam add-role-to-instance-profile --instance-profile-name tyche-ecs-instance --role-name tyche-ecs-instance
  sleep 10 # the profile isn't usable by run-instances right away
fi

# --- Data volume -----------------------------------------------------------
VOLUME_ID=$(aws ec2 describe-volumes \
  --filters Name=tag:Name,Values=tyche-data Name=availability-zone,Values="$AZ" \
  --query 'Volumes[0].VolumeId' --output text)
if none "$VOLUME_ID"; then
  log "Creating the data volume"
  VOLUME_ID=$(aws ec2 create-volume --availability-zone "$AZ" --size "$DATA_SIZE_GB" \
    --volume-type gp3 --encrypted \
    --tag-specifications 'ResourceType=volume,Tags=[{Key=Name,Value=tyche-data},{Key=Backup,Value=tyche}]' \
    --query VolumeId --output text)
  aws ec2 wait volume-available --volume-ids "$VOLUME_ID"
fi

# --- Instance --------------------------------------------------------------
INSTANCE_ID=$(aws ec2 describe-instances \
  --filters Name=tag:Name,Values=tyche Name=instance-state-name,Values=pending,running,stopping,stopped \
  --query 'Reservations[0].Instances[0].InstanceId' --output text)
if none "$INSTANCE_ID"; then
  log "Launching the $INSTANCE_TYPE instance"
  AMI_ID=$(aws ssm get-parameter \
    --name /aws/service/ecs/optimized-ami/amazon-linux-2023/arm64/recommended/image_id \
    --query Parameter.Value --output text)
  ROOT_DEVICE=$(aws ec2 describe-images --image-ids "$AMI_ID" --query 'Images[0].RootDeviceName' --output text)
  USER_DATA=$(mktemp)
  sed "s/__VOLUME_ID__/$VOLUME_ID/" user-data.sh >"$USER_DATA"
  INSTANCE_ID=$(aws ec2 run-instances --image-id "$AMI_ID" --instance-type "$INSTANCE_TYPE" \
    --subnet-id "$SUBNET_ID" --security-group-ids "$SG_ID" \
    --iam-instance-profile Name=tyche-ecs-instance \
    --user-data "file://$USER_DATA" \
    --block-device-mappings "DeviceName=$ROOT_DEVICE,Ebs={VolumeSize=30,VolumeType=gp3,Encrypted=true,DeleteOnTermination=true}" \
    --metadata-options HttpTokens=required,HttpEndpoint=enabled \
    --disable-api-termination \
    --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=tyche}]' \
    --query 'Instances[0].InstanceId' --output text)
  rm -f "$USER_DATA"
  aws ec2 wait instance-running --instance-ids "$INSTANCE_ID"
fi

ATTACHED_TO=$(aws ec2 describe-volumes --volume-ids "$VOLUME_ID" --query 'Volumes[0].Attachments[0].InstanceId' --output text)
if none "$ATTACHED_TO"; then
  log "Attaching the data volume"
  aws ec2 attach-volume --volume-id "$VOLUME_ID" --instance-id "$INSTANCE_ID" --device /dev/sdf >/dev/null
  aws ec2 wait volume-in-use --volume-ids "$VOLUME_ID"
elif [ "$ATTACHED_TO" != "$INSTANCE_ID" ]; then
  echo "tyche-data is attached to $ATTACHED_TO, not $INSTANCE_ID" >&2
  exit 1
fi

ALLOC_ID=$(aws ec2 describe-addresses --filters Name=tag:Name,Values=tyche \
  --query 'Addresses[0].AllocationId' --output text)
if none "$ALLOC_ID"; then
  log "Allocating the Elastic IP"
  ALLOC_ID=$(aws ec2 allocate-address --domain vpc \
    --tag-specifications 'ResourceType=elastic-ip,Tags=[{Key=Name,Value=tyche}]' \
    --query AllocationId --output text)
fi
aws ec2 associate-address --allocation-id "$ALLOC_ID" --instance-id "$INSTANCE_ID" >/dev/null
PUBLIC_IP=$(aws ec2 describe-addresses --allocation-ids "$ALLOC_ID" --query 'Addresses[0].PublicIp' --output text)

# --- ECR, logs, backups ----------------------------------------------------
if ! aws ecr describe-repositories --repository-names tyche >/dev/null 2>&1; then
  log "Creating the ECR repository"
  aws ecr create-repository --repository-name tyche --image-scanning-configuration scanOnPush=true >/dev/null
fi
aws ecr put-lifecycle-policy --repository-name tyche --lifecycle-policy-text file://ecr-lifecycle.json >/dev/null
aws logs create-log-group --log-group-name /ecs/tyche 2>/dev/null || true
aws logs put-retention-policy --log-group-name /ecs/tyche --retention-in-days 30

if ! aws iam get-role --role-name AWSDataLifecycleManagerDefaultRole >/dev/null 2>&1; then
  aws dlm create-default-role --resource-type snapshot >/dev/null
  sleep 10
fi
POLICY_ID=$(aws dlm get-lifecycle-policies --target-tags Backup=tyche --query 'Policies[0].PolicyId' --output text)
POLICY_DETAILS=$(sed "s/__COPY_REGION__/$COPY_REGION/" dlm-policy.json)
if none "$POLICY_ID"; then
  log "Creating the snapshot policy"
  aws dlm create-lifecycle-policy --description "tyche data volume daily snapshots" --state ENABLED \
    --execution-role-arn "arn:aws:iam::$ACCOUNT:role/AWSDataLifecycleManagerDefaultRole" \
    --policy-details "$POLICY_DETAILS" >/dev/null
else
  aws dlm update-lifecycle-policy --policy-id "$POLICY_ID" --policy-details "$POLICY_DETAILS"
fi

# --- ECS -------------------------------------------------------------------
aws ecs describe-clusters --clusters "$CLUSTER" --query 'clusters[?status==`ACTIVE`].clusterName' --output text | grep -q . ||
  aws ecs create-cluster --cluster-name "$CLUSTER" >/dev/null

if [ -z "$IMAGE" ] && ! aws ecs describe-task-definition --task-definition "$FAMILY" >/dev/null 2>&1; then
  echo "no $FAMILY yet, pass an image: aws/infra/up.sh $ACCOUNT.dkr.ecr.$REGION.amazonaws.com/tyche:<tag>" >&2
  exit 1
fi
if [ -n "$IMAGE" ]; then
  log "Registering $FAMILY with $IMAGE"
  TASK_DEF_FILE=$(mktemp)
  sed -e "s|__IMAGE__|$IMAGE|" -e "s|__ACCOUNT__|$ACCOUNT|g" -e "s|__REGION__|$REGION|g" \
    -e "s|__DOMAIN__|$DOMAIN|g" \
    task-definition.json >"$TASK_DEF_FILE"
  aws ecs register-task-definition --cli-input-json "file://$TASK_DEF_FILE" >/dev/null
  rm -f "$TASK_DEF_FILE"
fi
TASK_DEF=$(aws ecs describe-task-definition --task-definition "$FAMILY" --query taskDefinition.taskDefinitionArn --output text)

SERVICE_STATUS=$(aws ecs describe-services --cluster "$CLUSTER" --services "$SERVICE" --query 'services[0].status' --output text 2>/dev/null || true)
if [ "$SERVICE_STATUS" != "ACTIVE" ]; then
  log "Creating $SERVICE"
  aws ecs create-service --cluster "$CLUSTER" --service-name "$SERVICE" --task-definition "$TASK_DEF" \
    --desired-count 1 --launch-type EC2 \
    --deployment-configuration 'minimumHealthyPercent=0,maximumPercent=100,deploymentCircuitBreaker={enable=true,rollback=true}' \
    >/dev/null
elif [ -n "$IMAGE" ]; then
  aws ecs update-service --cluster "$CLUSTER" --service "$SERVICE" --task-definition "$TASK_DEF" --desired-count 1 >/dev/null
fi

log "Done"
echo "instance:    $INSTANCE_ID"
echo "data volume: $VOLUME_ID"
echo "public ip:   $PUBLIC_IP  (point the A record of $DOMAIN here)"
