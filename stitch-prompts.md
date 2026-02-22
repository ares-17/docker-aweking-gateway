Crea il design di un'interfaccia web responsive per un progetto chiamato "Docker Awakening Gateway". Si tratta di un gateway intelligente per container Docker che funziona così:

🏗️ **CONTESTO E FUNZIONALITÀ**
- È un servizio che intercetta le richieste HTTP verso container Docker che possono essere spenti o già attivi
- Se il container richiesto è SPENTO: l'utente vede una pagina di "attesa/risveglio" con un'animazione e un messaggio chiaro
- Se il container è già ATTIVO: l'utente viene reindirizzato direttamente al servizio senza vedere alcuna interfaccia intermedia
- Il sistema deve essere EXTREMAMENTE LEGGERO (minimo consumo RAM/CPU) e sicuro

🎨 **PAGINA DI ATTESA (quando il container è spento)**

Deve includere questi elementi:

1. **Header minimale** (solo logo/servizio e stato)
   - Logo: un'icona stilizzata di un container Docker che si "risveglia" (es. container con linee ondulate o occhi che si aprono)
   - Badge di stato: "Avvio container in corso..." con pallino animato

2. **Area principale** (centrata, quasi zen)
   - Animazione minimal: un container (rettangolo con bordi arrotondati) che "vibra" delicatamente o ha un effetto "respiro" (pulsazione leggera)
   - Messaggio principale: "Stiamo risvegliando il container [NOME_CONTAINER]"
   - Sottotitolo: "L'operazione richiede solo pochi secondi. Non ricaricare la pagina."
   - Indicatore di progresso: una barra di caricamento estremamente sottile (2px) che si riempie dolcemente

3. **Footer invisibile** (solo informazioni tecniche minime in grigio chiaro)
   - "Docker Awakening Gateway • Istanza: [ID] • Richiesta: [PATH]"

🎯 **COMPORTAMENTO E INTERAZIONI**

- La pagina NON deve avere pulsanti cliccabili (l'utente deve solo attendere)
- Dopo l'avvio del container, il reindirizzamento deve essere automatico (ma questo lo gestisce il backend, non la UI)
- Se l'avvio fallisce (opzionale): mostrare un messaggio di errore gentile con suggerimento di riprovare

📱 **RESPONSIVITÀ**

- Desktop: layout centrato con larghezza massima 600px, animazione di dimensioni moderate
- Tablet: simile a desktop, leggermente più compatto
- Mobile: animazione più piccola (risparmio energetico), testo ridimensionato, padding adeguato

🎭 **MOOD E STILE**

- **Estremamente leggero** = niente gradienti pesanti, niente ombre profonde, niente immagini pesanti
- **Palette colori**: bianco/grigio chiarissimo come base, accento colore tenue (blu/grigio-blu o verde salvia) per gli elementi animati
- **Tipografia**: font sans-serif moderno e leggibile (es. Inter, system-ui)
- **Animazioni**: appena accennate, quasi impercettibili ma presenti - l'effetto deve essere "calmo" non "frenetico"
- **Estetica**: "funzionale con un tocco umano" - sembra un pezzo di infrastruttura ma curato nei dettagli

🔧 **CONSIDERAZIONI TECNICHE PER IL DESIGN**

- L'interfaccia deve poter essere servita da un container Docker estremamente piccolo (ideale: immagine Alpine con Nginx o anche un server HTTP in Go/Rust)
- Zero JavaScript pesante, zero framework frontend - solo HTML e CSS minimale
- Se possibile, usare variabili CSS per personalizzazione futura
- L'animazione deve funzionare senza JS (solo CSS) per la parte "respiro", ma con JS si può arricchire (es. progresso simulato)

📐 **LAYOUT SPECIFICO**

```
┌─────────────────────────────────────┐
│  ┌───┐ Nome Servizio        ●●●     │  Header
│  └───┘                              │
├─────────────────────────────────────┤
│                                     │
│            ┌──────────┐             │
│            │  ░░░░░░   │             │  Animazione container
│            │  ░░░░░░   │             │  (effetto respiro)
│            └──────────┘             │
│                                     │
│   Stiamo risvegliando il container  │
│           [nome-container]          │
│                                     │
│   L'operazione richiede solo pochi  │
│   secondi. Non ricaricare la pagina.│
│                                     │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │  Barra progresso
│                                     │
├─────────────────────────────────────┤
│  Docker Awakening Gateway • v0.1.0  │  Footer
└─────────────────────────────────────┘
```

✨ **SPAZIO ALLA CREATIVITÀ**

- Senti libero di interpretare l'animazione del "risveglio" in modi sottili e ingegnosi
- Puoi giocare con micro-interazioni quando la pagina viene aperta
- Se hai idee per rendere l'attesa piacevole (quasi meditativa) invece che frustrante, ben vengano

🎁 **EXTRAS (SE AVANZA SPAZIO CREATIVO)**

- Versione dark mode (opzionale, attivabile da prefers-color-scheme)
- Una versione "ultra-minimal" con solo testo e barra (per ambienti embedded)
- Un easter egg sottilissimo: forse il container nell'animazione potrebbe "aprire gli occhi" quando è pronto?

🎯 **OBIETTIVO FINALE**

Una UI che sembra "parte dell'infrastruttura" ma curata, che rassicura l'utente tecnico durante l'attesa e che pesa zero in termini di risorse. Deve comunicare: "Sto lavorando, torna subito tutto operativo" senza ansia.