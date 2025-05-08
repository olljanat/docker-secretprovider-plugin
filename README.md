# About
Secrets plugin for Docker with multiple backends and auto rotation support.

Supported backends:
* [Azure Key Vault](https://azure.microsoft.com/en-us/products/key-vault/)
* [HashiCorp Vault](https://www.hashicorp.com/en/products/vault)
* [Passwordstate](https://www.clickstudios.com.au/passwordstate.aspx)

**TODO**
* Run docker-secretprovider-plugin.exe as service in Windows
* Save info about added secrets or even create them automatically based on backend content?
* Store config to secured JSON file before plugin install

# Usage
## Linux
```bash
docker volume create --driver secret secret-test1
docker run -it --rm -u nobody -v secret-test1:/secrets/test1 bash
cat /secrets/test1
```

## Windows
```powershell
docker volume create --driver secret secret-test1
docker volume ls
docker run -it --rm -v secret-test1:C:\secrets\test1 mcr.microsoft.com/windows/nanoserver:ltsc2022
type C:\secrets\test1

docker run -it --rm -v secret-test1:C:\ProgramData\Docker\secrets\test1 mcr.microsoft.com/windows/nanoserver:ltsc2022
type C:\ProgramData\Docker\secrets\test1
```

# Installation
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
  ollijanatuinen/docker-secretprovider-plugin:v0.1 \
  SECRET_BACKEND="azure" \
  AZURE_TENANT_ID="13a69a3b-cf5f-4204-b274-3e9ce5240a60" \
  AZURE_CLIENT_ID="2bb1a59c-72c5-4fba-81b3-f22974dfdf58" \
  AZURE_CLIENT_SECRET="" \
  AZURE_KEYVAULT_URL="https://dockersecret.vault.azure.net"
```
### Windows
```powershell
$env:SECRET_BACKEND="azure"
$env:AZURE_TENANT_ID="13a69a3b-cf5f-4204-b274-3e9ce5240a60"
$env:AZURE_CLIENT_ID="2bb1a59c-72c5-4fba-81b3-f22974dfdf58"
$env:AZURE_CLIENT_SECRET=""
$env:AZURE_KEYVAULT_URL="https://dockersecret.vault.azure.net"
.\docker-secretprovider-plugin.exe
```

## HashiCorp Vault
* Deploy Vault (e.g. `docker run -it --rm -p 8200:8200 --name=dev-vault hashicorp/vault`)
* Add dedicated engine for this use case
  * Secrets Engines -> Enable new engine -> Generic: KV -> Path: `docker`
* Add test secret under engine "docker"
  * Path for this secret: `test1`
  * Secret data -> key = **secret** (hardcoded value because of compatibility with other backends)
* Install plugin to servers like described below.
### Linux
```bash
docker plugin install \
  --alias secret \
  --grant-all-permissions \
  ollijanatuinen/docker-secretprovider-plugin:v0.1 \
  SECRET_BACKEND="vault" \
  VAULT_ADDR="http://100.64.255.102:8200" \
  VAULT_PATH="docker" \
  VAULT_TOKEN=""
```
### Windows
```powershell
$env:SECRET_BACKEND="vault"
$env:VAULT_ADDR="http://100.64.255.102:8200"
$env:VAULT_PATH="docker"
$env:VAULT_TOKEN=""
.\docker-secretprovider-plugin.exe
```

## Passwordstate
* Create list for this usage
* Create API key
### Linux
```bash
docker plugin install \
  --alias secret \
  --grant-all-permissions \
  ollijanatuinen/docker-secretprovider-plugin:v1.0 \
  PASSWORDSTATE_BASE_URL="https://passwordstate/api" \
  PASSWORDSTATE_API_KEY="<api key>" \
  PASSWORDSTATE_LIST_ID="123"
```
### Windows
```powershell
$env:SECRET_BACKEND="passwordstate"
$env:PASSWORDSTATE_BASE_URL="https://passwordstate/api"
$env:PASSWORDSTATE_API_KEY="<api key>"
$env:PASSWORDSTATE_LIST_ID="485"
.\docker-secretprovider-plugin.exe
```
