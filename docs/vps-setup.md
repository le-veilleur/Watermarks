# Setup VPS — Watermark

À faire **une seule fois** manuellement sur le VPS. Ensuite le CD gère tout automatiquement.

---

## 1. Créer les dossiers

```bash
mkdir ~/Watermarks
mkdir ~/Watermarks/secrets
cd ~/Watermarks
```

---

## 2. Créer les secrets

Ces fichiers ne sont **jamais committé** (gitignorés). À créer manuellement sur le VPS.

```bash
# Identifiants MinIO (choisir librement)
echo "ton_user_minio" > secrets/minio_user.txt
echo "ton_password_minio" > secrets/minio_password.txt

# Identifiants RabbitMQ
echo "default_user = ton_user_rabbit"    > secrets/rabbitmq.conf
echo "default_pass = ton_password_rabbit" >> secrets/rabbitmq.conf
```

---

## 3. S'authentifier sur GHCR

Permet à Docker de puller les images privées depuis GitHub Container Registry.

**Créer un token GitHub :**
- GitHub → Settings → Developer settings → Personal access tokens → Tokens (classic)
- Scope requis : `read:packages` uniquement
- Copier le token généré (`ghp_xxx...`)

**Se connecter sur le VPS :**

```bash
echo "ghp_xxx..." | docker login ghcr.io -u le-veilleur --password-stdin
# → Login Succeeded
```

Les credentials sont sauvegardés dans `~/.docker/config.json`. À ne faire qu'une seule fois.

---

## 4. Secrets GitHub Actions

À ajouter dans le repo → Settings → Secrets and variables → Actions :

| Nom | Valeur |
|-----|--------|
| `VPS_HOST` | IP du VPS (ex: `51.83.46.132`) |
| `VPS_USER` | Utilisateur SSH (ex: `debian`) |
| `VPS_SSH_KEY` | Clé privée SSH (contenu de `~/.ssh/id_rsa`) |

---

## Ce que fait le CD ensuite (automatique)

À chaque merge sur `main` :
1. Retague les images `:devlop` → `:latest` sur GHCR
2. Copie le `docker-compose.yml` sur le VPS via SCP
3. SSH → `docker compose pull` + `docker compose up -d`

Aucune intervention manuelle nécessaire.

---

## Comment ça marche (résumé)

```
GitHub Actions
  → compile le code Go dans une image Docker
  → push l'image sur GHCR (image publique, sans secrets)

VPS
  → docker compose pull   — télécharge l'image depuis GHCR
  → docker compose up     — crée le conteneur
                            ↓
                  monte ~/Watermarks/secrets/minio_user.txt
                  dans /run/secrets/minio_user à l'intérieur du conteneur
```

**Pourquoi les secrets ne sont pas dans l'image ?**
L'image est publique sur GHCR — n'importe qui peut la puller.
Les secrets restent uniquement sur le VPS et sont injectés au démarrage du conteneur par Docker.
L'app Go lit ensuite `/run/secrets/minio_user` pour récupérer la valeur.
