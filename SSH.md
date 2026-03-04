# Configuration SSH pour le déploiement CI/CD

Guide pour configurer la connexion SSH entre GitHub Actions et un VPS Debian.

---

## 1. Générer une clé SSH dédiée au CI

Sur **ton PC** :

```bash
ssh-keygen -t ed25519 -C "github-actions" -f ~/.ssh/github_actions -N ""
```

Génère deux fichiers :

| Fichier | Rôle |
|---|---|
| `~/.ssh/github_actions` | Clé privée → GitHub Secret `VPS_SSH_KEY` |
| `~/.ssh/github_actions.pub` | Clé publique → à coller sur le VPS |

---

## 2. Ajouter la clé publique sur le VPS

Sur **ton PC**, affiche la clé publique :

```bash
cat ~/.ssh/github_actions.pub
```

Sur **ton VPS** :

```bash
echo "COLLE_ICI_LE_CONTENU_DE_github_actions.pub" >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
```

---

## 3. Ajouter les secrets dans GitHub

```
Repo GitHub → Settings → Secrets and variables → Actions → New repository secret
```

| Secret | Valeur | Où la trouver |
|---|---|---|
| `VPS_HOST` | IP de ton VPS | Panel OVH / provider |
| `VPS_USER` | `debian` | Ton VPS |
| `VPS_SSH_KEY` | `cat ~/.ssh/github_actions` | Ton PC |

---

## 4. Tester la connexion avant de relancer le pipeline

Sur **ton PC** :

```bash
ssh -i ~/.ssh/github_actions debian@IP_DU_VPS "echo OK"
```

Si la commande affiche `OK` → les secrets sont corrects, le pipeline passera.

---

## 5. Erreurs fréquentes

### `ssh: no key found`
La clé dans `VPS_SSH_KEY` est vide ou mal copiée.
→ Recopier le contenu **complet** de `cat ~/.ssh/github_actions`, headers inclus :

```
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAA...
-----END OPENSSH PRIVATE KEY-----
```

### `attempted methods [none publickey]`
La clé publique n'est pas dans `authorized_keys` sur le VPS, ou ne correspond pas.
→ Vérifier que les deux sont identiques :

```bash
# Sur ton PC
cat ~/.ssh/github_actions.pub

# Sur le VPS
cat ~/.ssh/authorized_keys
```

Les deux doivent afficher la même ligne `ssh-ed25519 AAAA... github-actions`.

### `Connection refused`
Le port SSH est bloqué ou l'IP dans `VPS_HOST` est incorrecte.
→ Tester manuellement :

```bash
ssh -i ~/.ssh/github_actions -v debian@IP_DU_VPS
```

---

## 6. Schéma récapitulatif

```
TON PC                        GITHUB SECRETS              VPS
──────                        ──────────────              ───
github_actions      ───────→  VPS_SSH_KEY
github_actions.pub  ───────────────────────────────────→  ~/.ssh/authorized_keys
IP du VPS           ───────→  VPS_HOST
"debian"            ───────→  VPS_USER
```
