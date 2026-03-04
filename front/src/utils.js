// Même logique que formatBytes côté Go — dupliquée ici pour éviter un appel API juste pour l'affichage
export function formatBytes(bytes) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(2) + ' MB'
}

// Convertit les millisecondes renvoyées par les headers X-T-* en chaîne lisible
export function formatMs(ms) {
  if (ms === null || ms === undefined) return null // header absent = étape ignorée (ex: MinIO sur cache HIT)
  const n = parseFloat(ms)                         // les headers arrivent en string
  if (n >= 1000) return (n / 1000).toFixed(2) + ' s'  // au-delà d'une seconde, afficher en s pour lisibilité
  if (n >= 1)    return n.toFixed(1) + ' ms'
  return n.toFixed(2) + ' ms'                          // en dessous de 1ms (ex: hash), garder 2 décimales
}

// Construit la liste des étapes du pipeline à partir des headers X-T-* exposés par l'API.
// Chaque header présent = l'étape a été exécutée ; absent = court-circuitée (ex: cache HIT saute l'optimizer).
export function parsePipeline(headers) {
  const steps = []
  const h = (k) => headers.get(k) // raccourci pour éviter la répétition de headers.get(...)

  if (h('X-T-Read'))      steps.push({ key: 'Read',      ms: h('X-T-Read'),      status: 'ok' })
  if (h('X-T-Hash'))      steps.push({ key: 'Hash',      ms: h('X-T-Hash'),      status: 'ok' })
  // Redis est la seule étape avec trois états : hit (servi depuis le cache), miss (clé absente), ou absent (non exécuté)
  if (h('X-T-Redis'))     steps.push({ key: 'Redis',     ms: h('X-T-Redis'),     status: h('X-Cache') === 'HIT' ? 'hit' : 'miss' })
  if (h('X-T-Minio'))     steps.push({ key: 'MinIO',     ms: h('X-T-Minio'),     status: 'ok' })
  if (h('X-T-Optimizer')) steps.push({ key: 'Optimizer', ms: h('X-T-Optimizer'), status: 'ok' })
  if (h('X-T-Store'))     steps.push({ key: 'Store',     ms: h('X-T-Store'),     status: 'ok' })
  // X-T-Rabbit présent = l'optimizer était KO, le job est en queue → statut spécial orange
  if (h('X-T-Rabbit'))    steps.push({ key: 'RabbitMQ',  ms: h('X-T-Rabbit'),    status: 'rabbit' })

  return steps.length > 0 ? steps : null // null = pas de headers de timing = réponse sans pipeline (erreur)
}
