# About
Secrets plugin for Docker with multiple backends and auto rotation support.

Supported backends:
* [Azure Key Vault](https://azure.microsoft.com/en-us/products/key-vault/)
* [HashiCorp Vault](https://www.hashicorp.com/en/products/vault)
* [Passwordstate](https://www.clickstudios.com.au/passwordstate.aspx)

**NOTE!!!** Please, make sure that you always use long format of --mount command with `volume-driver=secret` parameter.
Other why you might end up to have local volume with that name instead of.

# Usage
## Linux
```bash
docker run -it --rm -u nobody \
 --mount type=volume,volume-driver=secret,src=test1,dst=/secrets/test1,readonly \
  bash
cat /secrets/test1
```

## Windows
```powershell
docker volume ls
docker run -it --rm `
  --mount type=volume,volume-driver=secret,src=test1,dst=C:\secrets\test1,readonly `
  mcr.microsoft.com/windows/nanoserver:ltsc2022
type C:\secrets\test1
```

# Installation
Windows binaries are published under releases. Linux plugins can installed directly from Docker Hub like described below.

## Windows
Installation commands are same for all backends in Windows so it described here just once.
```powershell
# Copy binary and create service
Copy-Item -Path docker-secretprovider-plugin.exe -Destination "C:\Program Files\docker"
New-Service -Name "docker-secret" -DisplayName "Secrets plugin for Docker" `
  -BinaryPathName "C:\Program Files\docker\docker-secretprovider-plugin.exe" -StartupType Automatic

# Register eventlog handler
$log = "HKLM:\SYSTEM\CurrentControlSet\Services\EventLog\Application\docker-secret"
New-Item -Path $log -Force
Set-ItemProperty -Path $log -Name CustomSource -Value 1
Set-ItemProperty -Path $log -Name EventMessageFile -Value "%SystemRoot%\System32\EventCreate.exe"
Set-ItemProperty -Path $log -Name TypesSupported -Value 7

# Make Docker service depend on of plugin service
## Please note that you need reboot server before this is effective.
Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\docker" `
  -Name DependOnService -Type MultiString -Value @("docker-secret")
```
Check configuration guide under each backend section.


## Azure Key Vault
* Create service principal with long enough validity period.
```bash
az ad sp create-for-rbac -n docker-secretprovider-plugin --years 5
```
**NOTE!!!** Because this service principal is valid long time, it is very important that it only has access right to this Key Vault and that IP restrictions are used.

* Create dedicated Azure Key Vault following settings:
  * Permission model: Vault access policy
    * Secret permissions `Get` and `List` assigned for service principal `docker-secretprovider-plugin`
    * Secret permissions `Set` assigned for users/automations which create and update secrets.
  * Enable public access: Enabled
  * Allow public access: Allow public access from specific virtual networks and IP addresses
    * List virtual networks containing Azure VMs using this plugin.
    * List public IPs of non-Azure VMs using this plugin.
* Add test secret to Key Vault.
* Install plugin to servers like described below.

### Linux
```bash
docker plugin install \
  --alias secret \
  --grant-all-permissions \
  ollijanatuinen/docker-secretprovider-plugin:v1.0 \
  SECRET_BACKEND="azure" \
  AZURE_TENANT_ID="13a69a3b-cf5f-4204-b274-3e9ce5240a60" \
  AZURE_CLIENT_ID="2bb1a59c-72c5-4fba-81b3-f22974dfdf58" \
  AZURE_CLIENT_SECRET="<secret>" \
  AZURE_KEYVAULT_URL="https://dockersecret.vault.azure.net"
```

### Windows
```powershell
# Add environment variables for service
Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\docker-secret" `
  -Name Environment `
  -Type MultiString `
  -Value @(
  "SECRET_BACKEND=azure",
  "AZURE_TENANT_ID=13a69a3b-cf5f-4204-b274-3e9ce5240a60",
  "AZURE_CLIENT_ID=2bb1a59c-72c5-4fba-81b3-f22974dfdf58",
  "AZURE_CLIENT_SECRET=<secret>",
  "AZURE_KEYVAULT_URL=https://dockersecret.vault.azure.net"
)
```

## HashiCorp Vault
* Deploy Vault (e.g. `docker run -it --rm -p 8200:8200 --name=dev-vault hashicorp/vault`)
* Add dedicated engine for this use case
  * Secrets Engines -> Enable new engine -> Generic: KV -> Path: `docker`
* Add test secret under engine "docker"
  * Path for this secret: `test1`
  * Secret data -> key = **Secret** (hardcoded value because of compatibility with other backends)
  * Custom metadata -> key = **ExpiryDate** in format `yyyy-MM-DD` (hardcoded value because of compatibility with other backends)
* Install plugin to servers like described below.

### Linux
```bash
docker plugin install \
  --alias secret \
  --grant-all-permissions \
  ollijanatuinen/docker-secretprovider-plugin:v1.0 \
  SECRET_BACKEND="vault" \
  VAULT_ADDR="http://10.10.10.100:8200" \
  VAULT_PATH="docker" \
  VAULT_TOKEN="<token"
```

### Windows
```powershell
# Add environment variables for service
Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\docker-secret" `
  -Name Environment `
  -Type MultiString `
  -Value @(
  "SECRET_BACKEND=vault",
  "VAULT_ADDR=http://10.10.10.100:8200",
  "VAULT_PATH=docker",
  "VAULT_TOKEN=<token>"
)
```


## Passwordstate
* Create list for this usage
* Create API key
* Configure IP restrictions for API key

### Linux
```bash
docker plugin install \
  --alias secret \
  --grant-all-permissions \
  ollijanatuinen/docker-secretprovider-plugin:v1.0 \
  SECRET_BACKEND="passwordstate" \
  PASSWORDSTATE_BASE_URL="https://passwordstate/api" \
  PASSWORDSTATE_API_KEY="<api key>" \
  PASSWORDSTATE_LIST_ID="123"
```

### Windows
```powershell
# Add environment variables for service
Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\docker-secret" `
  -Name Environment `
  -Type MultiString `
  -Value @(
  "SECRET_BACKEND=passwordstate",
  "PASSWORDSTATE_BASE_URL=https://passwordstate/api",
  "PASSWORDSTATE_API_KEY=<api key>",
  "PASSWORDSTATE_LIST_ID=485"
)
```

# Troubleshooting
If secrets plugin writes events to:
* Windows event log with provider name `docker-secret`
* To Docker engine log ( `/var/log/docker.log` ) with plugin ID.

Most common issue is that some of the environment variables is missing and contains incorrect value.
