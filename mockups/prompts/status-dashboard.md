Crea il design di un'interfaccia web responsive per il progetto "Docker Awakening Gateway". Si tratta di una **pagina dashboard di monitoraggio** che mostra in tempo reale lo stato di tutti i container Docker gestiti dal gateway.

---

🏗️ **CONTESTO E FUNZIONALITÀ**

- Il gateway gestisce N container Docker (tipicamente 2–10 in un home lab / edge server)
- Questa pagina mostra una panoramica unificata dello stato di ciascuno
- I container possono trovarsi in questi stati: **running** (attivo), **stopped/exited** (spento), **starting** (in avvio), **failed** (errore), **idle** (in procinto di essere spento per inattività)
- La pagina è raggiungibile all'endpoint interno `/_status` del gateway
- NON è una pagina pubblica: è per l'operatore tecnico che amministra il sistema

---

🎨 **STRUTTURA DELLA PAGINA**

1. **Header**
   - Logo/nome "Docker Awakening Gateway" a sinistra con piccola icona container
   - Badge compatto con il numero totale di container: es. "5 containers • 3 running • 1 idle • 1 stopped"
   - Data/ora ultimo aggiornamento (es. "Last updated: 11:27:03") — aggiornata ogni ~5s
   - Dark mode toggle in alto a destra

2. **Griglia di card container** (layout a colonna singola su mobile, 2 colonne su tablet, sino a 3 su desktop wide)

   Ogni card deve contenere:

   a) **Intestazione card**
      - Nome container (font monospace in evidenza, es. `slow-app`)
      - Badge stato colorato (pillola): verde "running", arancio "starting", rosso "failed", grigio "stopped", giallo/ambra "idle"
      - Icona piccola di container Docker stilizzata a sinistra del nome

   b) **Barra di uptime/status (elemento principale e distintivo)**
      - Una riga di **stecchette rettangolari con bordi arrotondati** (tipo "Heartbeat bars" dei servizi di monitoring come Betterstack, Upptime, StatusPage)
      - Circa 30–40 stecchette orizzontali, ognuna rappresenta un intervallo di tempo recente (es. 1 min cadauna = ultimi 30-40 min)
      - Colori delle stecchette:
        - **Verde** (#22c55e o simile): container era running in quell'intervallo
        - **Rosso** (#ef4444): container era in errore / crashed
        - **Grigio** (#6b7280): container era spento/offline
        - **Arancio/Giallo** (#f59e0b): container era in avvio (starting)
        - **Blu** (#3b82f6): container era in riavvio per idle (awakening)
      - La stecchetta più recente è a destra
      - Al passaggio del mouse (hover) su ogni stecchetta: tooltip con timestamp e stato in quell'intervallo
      - Sotto la barra: etichetta discreta "30 min ago" a sinistra e "now" a destra

   c) **Metriche sintetiche** (riga sotto la barra)
      - Uptime corrente: es. "↑ 2h 14m" oppure "↓ Stopped 5m ago"
      - Ultima attività: es. "Last request: 3m ago"
      - Timeout configurato: es. "Idle timeout: 10m"
      - Porta target: es. "→ :8080"

   d) **Footer card** (molto discreto)
      - `idle_timeout`, `start_timeout`, e `network` se configurato
      - Link rapido "Wake now" (bottone ghost piccolo) se il container è spento

3. **Footer pagina**
   - "Docker Awakening Gateway • /_status • v0.2.0"
   - Refresh automatico indicato: "Auto-refresh: 5s"

---

🎯 **STATI SPECIALI DA MOSTRARE**

- **Container in avvio**: la stecchetta più recente è arancione + animazione pulse; il badge mostra "Starting…"
- **Container fallito**: ultime stecchette rosse + bordo card sottile rosso; badge "Failed" con icona warning
- **Container idle (in procinto di stop)**: badge giallo/ambra "Idle • stops in 3m"; barra con ultima stecchetta ambra
- **Container mai avviato** (nessuna storia): barre tutte grigie; sottotitolo "Never started"

---

📱 **RESPONSIVITÀ**

- Mobile (<768px): colonna singola, stecchette ridotte (20 visibili), metriche collassate in 2 righe
- Tablet: 2 colonne
- Desktop (>1280px): fino a 3 colonne; stecchette full (40)
- Le card hanno altezza fissa su desktop per allineamento griglia

---

🎭 **MOOD E STILE**

- **Palette dark mode** (default): sfondo `#0d1117`, card `#161b22`, bordi `#30363d` — coerente con l'estetica del resto del gateway (Precision Grid)
- **Palette light mode**: sfondo `#f8fafc`, card `#ffffff`, bordi `#e2e8f0`
- **Tipografia**: Space Grotesk per i titoli, JetBrains Mono per nomi container e valori tecnici
- **Stile generale**: "technical dashboard" — pulito, denso di informazioni ma non caotico; ispirato a tools come Grafana dark, Linear, Vercel dashboard
- **Animazioni**: minime — solo il badge "Starting" ha un pulse; le stecchette appaiono con una transizione lineare da sinistra a destra al caricamento della pagina
- **Nessuna decorazione superflua** — niente sfondi texture, niente gradienti aggressivi, niente icone decorative

---

🔧 **CONSIDERAZIONI TECNICHE**

- HTML + CSS + JS vanilla (no framework)
- Le stecchette sono `<div>` con `border-radius: 3px`, larghezza ~6px, altezza ~24px, gap ~2px
- Dati simulati nel mockup (array JS hardcodati per ogni container)
- I tooltip sulle stecchette sono CSS puri (`:hover + ::after`) per leggerezza
- Tailwind via CDN accettato per prototipazione rapida
- Deve essere possibile servire la pagina embeddanda i dati come JSON direttamente nel template Go (`go:embed`)

---

📐 **LAYOUT CARD (wireframe)**

```
┌──────────────────────────────────────────────┐
│  🐳 slow-app                    ● RUNNING    │
│                                              │
│  ▐▌▐▌▐▌▐▌▐▌░░░░▐▌▐▌▐▌▐▌▐▌▐▌▐▌▐▌▐▌▐▌▐▌▐▌▐▌  │  ← stecchette
│  30m ago                              now   │
│                                              │
│  ↑ 1h 42m   Last req: 2m ago   → :80        │
│  idle: 10m  start: 90s                      │
└──────────────────────────────────────────────┘

┌──────────────────────────────────────────────┐
│  🐳 fail-app                    ✖ FAILED     │  ← bordo rosso
│                                              │
│  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░▐▌▐▌██████   │
│  30m ago                              now   │
│                                              │
│  ↓ Failed 8s ago   Last req: 8s ago  → :80  │
│  start timeout: 8s                          │
└──────────────────────────────────────────────┘
```

---

✨ **SPAZIO ALLA CREATIVITÀ**

- Le stecchette possono avere altezze leggermente diverse per simulare un "heartbeat" più biologico (le running più alte, le offline piatte)
- Il toggle dark/light può avere una micro-animazione di transizione sui colori
- Un effetto "shimmer" sulle stecchette grayed-out (container mai avviato) è un tocco raffinato
- Se lo ritieni utile, aggiungi una riga di summary in cima alla griglia con mini sparkline aggregate

---

🎯 **OBIETTIVO FINALE**

Una dashboard che un DevOps/homelabber apra in un tab separato per avere un colpo d'occhio istantaneo sulla salute di tutti i suoi servizi. Deve comunicare: "So esattamente cosa sta succedendo in questo momento" — densa di informazioni ma mai caotica. L'estetica deve essere coerente con il resto del Docker Awakening Gateway (dark, tecnica, curata).

Genera entrambe le versioni: **dark mode** e **light mode** come schermate separate.
