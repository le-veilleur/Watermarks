# Déploiement

## Flux

```
devlop  →  CI (tests) → CD build → push :devlop
main    →  CD promote (:devlop → :latest) → deploy VPS
```

## Secrets GitHub requis

| Secret | Description |
|---|---|
| `VPS_HOST` | IP du serveur |
| `VPS_USER` | Utilisateur SSH (`debian`) |
| `VPS_SSH_KEY` | Clé privée SSH (`cat ~/.ssh/github_actions`) |

## Setup VPS (une seule fois)

```bash
# Ajouter la clé publique
echo "$(cat ~/.ssh/github_actions.pub)" >> ~/.ssh/authorized_keys

# Login GHCR pour puller les images
docker login ghcr.io
```

## Tester la connexion SSH

```bash
ssh -i ~/.ssh/github_actions debian@IP_DU_VPS "echo OK"
```
