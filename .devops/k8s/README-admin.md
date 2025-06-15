# Admin Configuration Setup

## Setting Up Admin Users

Admin users are configured via a Kubernetes Secret to keep the admin email list private.

### Setup Steps

1. **Create the admin secret file:**
   ```bash
   cd .devops/k8s/base/
   cp admin-secret.yaml.example admin-secret.yaml
   ```

2. **Encode your admin emails:**
   ```bash
   # For a single admin email
   echo -n "greg@hndrx.co" | base64
   
   # For multiple admin emails (comma-separated)
   echo -n "greg@hndrx.co,other@admin.com" | base64
   ```

3. **Edit admin-secret.yaml:**
   Replace `<BASE64_ENCODED_ADMIN_EMAILS_HERE>` with the base64 output from step 2.

4. **Apply the secret to your cluster:**
   ```bash
   kubectl apply -f admin-secret.yaml
   ```

5. **Restart the portal deployment:**
   ```bash
   kubectl rollout restart deployment/medisynth-portal -n medisynth-io
   ```

### Security Notes

- The `admin-secret.yaml` file is gitignored and should never be committed
- Only the `.example` template is committed to git
- Admin emails are stored base64 encoded in the Kubernetes secret
- Only users with the specified email addresses will have admin access

### Admin Features

Once configured, admin users will see:
- Purple avatar instead of blue in the UI
- "Admin" badge next to their name
- Access to admin-only routes (when implemented)

### Adding/Removing Admins

To modify the admin list:
1. Update the `ADMIN_EMAILS` value in `admin-secret.yaml`
2. Re-apply the secret: `kubectl apply -f admin-secret.yaml`
3. Restart the portal: `kubectl rollout restart deployment/medisynth-portal -n medisynth-io` 