<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- homeContent is an administrator-managed setting. -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            to="/desktop"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.desktopDownload')"
          >
            <Icon name="download" size="md" />
            <span class="hidden sm:inline">{{ t('home.desktopDownload') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme()"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img
          :src="siteLogo || '/logo.svg'"
          alt="Logo"
          class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- Default home: a particle universe with real content layered on top. -->
  <div v-else ref="host" class="home-site terminal-container" :class="{ 'motion-disabled': lowMotion, 'is-light': !isDark }" data-testid="orbit-home">
    <OrbitScene
      data-testid="orbit-scene"
      :dark="isDark"
      :paused="lowMotion"
      :chapters="chapters"
      :title="wordmark"
      @title-visible="titleVisible = $event"
    />
    <a class="home-skip" href="#home-main">{{ t('home.orbit.skip') }}</a>
    <div class="home-vignette" aria-hidden="true" />
    <div class="home-progress" aria-hidden="true"><span ref="progressBar" /></div>

    <header class="home-header" :class="{ 'is-compact': scrolled }">
      <div class="home-header-bar">
        <router-link to="/home" class="home-brand">
          <img v-if="siteLogo" :src="siteLogo" alt="" class="home-brand-img" />
          <svg v-else class="home-brand-mark" viewBox="0 0 96 96" fill="none" aria-hidden="true">
            <rect width="96" height="96" rx="26" class="mark-tile" />
            <circle cx="48" cy="48" r="18" stroke-width="7" class="mark-core" />
            <g transform="rotate(-35 48 48)"><ellipse cx="48" cy="48" rx="36" ry="14" stroke-width="3" class="mark-orbit" /></g>
            <circle cx="76" cy="29" r="5" class="mark-node" />
          </svg>
          <span class="home-brand-text"><strong>{{ siteName }}</strong><small>{{ t('home.orbit.hudB') }}</small></span>
        </router-link>
        <nav class="home-nav" :aria-label="t('home.orbit.navigation')">
          <a href="#connect" class="home-nav-link">{{ t('home.orbit.platform') }}</a>
          <a href="#clients" class="home-nav-link">{{ t('home.orbit.ecosystem') }}</a>
          <a href="#start" class="home-nav-link">{{ t('home.orbit.start') }}</a>
          <a href="#faq" class="home-nav-link">{{ t('home.orbit.faq') }}</a>
        </nav>
        <div class="home-actions">
          <LocaleSwitcher class="home-locale" />
          <button type="button" class="home-icon" :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="toggleTheme()">
            <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
          </button>
          <router-link class="home-login" :to="isAuthenticated ? dashboardPath : '/login'">{{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</router-link>
          <router-link v-magnetic class="home-cta" :to="primaryDestination">{{ primaryLabel }}<i aria-hidden="true">→</i></router-link>
        </div>
        <button class="home-menu-btn" type="button" :aria-label="t('common.toggleMenu')" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen" @keydown.esc="menuOpen = false">
          <span /><span />
        </button>
      </div>
      <div v-if="menuOpen" class="home-menu" @keydown.esc="menuOpen = false">
        <nav :aria-label="t('home.orbit.navigation')">
          <a href="#connect" @click="menuOpen = false">{{ t('home.orbit.platform') }}</a>
          <a href="#clients" @click="menuOpen = false">{{ t('home.orbit.ecosystem') }}</a>
          <a href="#start" @click="menuOpen = false">{{ t('home.orbit.start') }}</a>
          <a href="#faq" @click="menuOpen = false">{{ t('home.orbit.faq') }}</a>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" @click="menuOpen = false">{{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</router-link>
        </nav>
      </div>
    </header>

    <nav v-if="!narrow" class="home-rail" :aria-label="t('home.orbit.chapters')">
      <button
        v-for="(label, index) in railLabels"
        :key="index"
        type="button"
        class="home-rail-dot"
        :class="{ 'is-active': activeChapter === index }"
        :aria-current="activeChapter === index ? 'step' : undefined"
        @click="scrollToChapter(index)"
      >
        <span class="home-rail-label">{{ label }}</span><i aria-hidden="true" />
      </button>
    </nav>

    <CursorAura :disabled="lowMotion" />

    <main id="home-main" class="home-main">
      <section id="hero" class="home-chapter hero" data-chapter>
        <div class="home-hud" aria-hidden="true">
          <i class="is-tl" /><i class="is-tr" /><i class="is-bl" /><i class="is-br" />
          <span class="hud-a">{{ t('home.orbit.hudA') }}</span>
          <span class="hud-b">{{ t('home.orbit.hudB') }}</span>
        </div>
        <div class="home-container hero-inner">
          <p class="hero-kicker" data-reveal="fade" style="--d: 700ms"><i aria-hidden="true" />{{ t('home.orbit.kicker') }}<i aria-hidden="true" /></p>
          <h1 class="hero-wordmark" :class="{ 'is-webgl': titleVisible }">
            <span class="sr-only">{{ siteName }}</span>
            <span class="hero-word" aria-hidden="true" data-reveal="scale" style="--d: 500ms">{{ wordmark }}</span>
          </h1>
          <div class="hero-bottom">
            <div class="hero-copy">
              <SplitText
                tag="h2"
                class="hero-headline"
                :text="t('home.orbit.headline')"
                data-reveal
                style="--d: 1250ms"
              />
              <p v-if="customSubtitle" class="hero-subtitle" data-reveal style="--d: 1450ms">{{ customSubtitle }}</p>
              <p class="hero-lead" data-reveal style="--d: 1550ms">{{ t('home.orbit.lead') }}</p>
              <div class="hero-links btn-row" data-reveal style="--d: 1750ms">
                <router-link v-magnetic class="home-btn home-btn-primary" :to="primaryDestination">
                  {{ primaryLabel }}<i aria-hidden="true">→</i>
                </router-link>
                <a class="home-btn home-btn-ghost" href="#connect">{{ t('home.orbit.explore') }}</a>
              </div>
            </div>
          </div>
        </div>
        <a class="hero-scroll" href="#manifesto" data-reveal="fade" style="--d: 2200ms" @click.prevent="scrollToChapter(1)">
          <span class="scroll-pill" aria-hidden="true"><i /></span>
          <span>{{ t('home.orbit.scroll') }}</span>
        </a>
      </section>

      <section id="manifesto" class="home-chapter manifesto" data-chapter data-pin>
        <div class="home-pin">
          <div class="home-container manifesto-inner">
            <p class="eyebrow" data-reveal>{{ t('home.orbit.manifestoLabel') }}</p>
            <p class="manifesto-text" :style="{ '--n': manifestoTotal }">
              <span class="sr-only">{{ t('home.orbit.manifesto').replace(/\[\[|\]\]/g, '') }}</span>
              <span aria-hidden="true">
                <span
                  v-for="(token, index) in manifestoTokens"
                  :key="index"
                  :class="token.space ? 'is-space' : ['mu', { 'is-hl': token.highlight }]"
                  :style="token.space ? undefined : { '--i': token.index }"
                >{{ token.text }}</span>
              </span>
            </p>
            <p class="manifesto-sign" data-reveal><i aria-hidden="true" />{{ siteName }}</p>
          </div>
        </div>
      </section>

      <section id="connect" class="home-chapter connect" data-chapter data-scene>
        <span class="outline-word connect-bgword" aria-hidden="true">CONNECT</span>
        <div class="home-container split">
          <div class="connect-copy">
            <p class="eyebrow" data-reveal><b>01</b>{{ t('home.orbit.connectLabel') }}</p>
            <SplitText tag="h2" class="home-h2" :text="t('home.orbit.connectTitle')" data-reveal />
            <p class="home-lead" data-reveal style="--d: 180ms">{{ t('home.orbit.connectDescription') }}</p>
            <div class="chips" data-reveal style="--d: 280ms">
              <span class="chip"><i />{{ t('home.orbit.chipOpenai') }}</span>
              <span class="chip"><i />Anthropic Messages</span>
              <span class="chip"><i />Gemini</span>
              <span class="chip"><i />Responses API</span>
              <span class="chip"><i class="is-amber" />{{ t('home.orbit.chipStream') }}</span>
            </div>
            <div class="home-card endpoint" data-reveal style="--d: 380ms">
              <div class="endpoint-row endpoint-url">
                <span class="num">{{ t('home.orbit.endpoint') }}</span>
                <code class="mono">{{ baseUrl }}</code>
                <button type="button" class="endpoint-copy" @click="copyEndpoint">
                  {{ copied ? t('common.copied') : t('common.copy') }}
                </button>
              </div>
              <p class="endpoint-feedback" role="status">{{ copyError ? t('home.orbit.copyFailed') : t('home.orbit.endpointHint') }}</p>
              <div class="stats">
                <div class="stat"><strong>1</strong><span>{{ t('home.orbit.statEndpoint') }}</span></div>
                <div class="stat"><strong>3+</strong><span>{{ t('home.orbit.statFormats') }}</span></div>
                <div class="stat"><strong>∞</strong><span>{{ t('home.orbit.statCombos') }}</span></div>
              </div>
            </div>
          </div>
          <div class="constellation">
            <span
              v-for="node in constellation"
              :key="node.name"
              class="planet"
              :style="{ left: `${node.x}%`, top: `${node.y}%` }"
              :data-depth="node.depth"
            >
              <span class="float" :style="{ animationDelay: `${node.delay}s` }"><i aria-hidden="true" />{{ node.name }}</span>
            </span>
            <p class="constellation-note">{{ t('home.orbit.modelsNote') }}</p>
          </div>
        </div>
      </section>

      <section ref="capsSection" id="capabilities" class="home-chapter caps" data-chapter data-pin :style="{ '--shift': capsShift }">
        <div class="home-pin caps-pin">
          <div class="home-container caps-head">
            <div>
              <p class="eyebrow" data-reveal><b>02</b>{{ t('home.orbit.capsLabel') }}</p>
              <SplitText tag="h2" class="home-h2" :text="t('home.orbit.capsTitle')" data-reveal />
            </div>
            <div class="caps-meter" data-reveal style="--d: 200ms">
              <span>{{ t('home.orbit.capsCount') }}</span>
              <i aria-hidden="true"><b /></i>
              <small>{{ t('home.orbit.capsHint') }}</small>
            </div>
          </div>
          <div class="caps-viewport">
            <div ref="capsTrack" class="caps-track">
              <article v-for="(capability, index) in capabilities" :key="capability.key" v-tilt class="home-card cap">
                <div class="cap-top">
                  <span class="num">{{ String(index + 1).padStart(2, '0') }}</span>
                  <span class="cap-dot" aria-hidden="true" />
                </div>
                <div class="cap-visual" :class="`is-${capability.key}`" aria-hidden="true">
                  <svg v-if="capability.key === 'access'" viewBox="0 0 260 120">
                    <path class="hv-line" d="M10 22 C 90 22, 110 60, 150 60" />
                    <path class="hv-line" d="M10 60 L 150 60" />
                    <path class="hv-line" d="M10 98 C 90 98, 110 60, 150 60" />
                    <path class="hv-flow" d="M150 60 L 250 60" />
                    <circle class="hv-node" cx="150" cy="60" r="11" /><circle class="hv-core" cx="150" cy="60" r="4" />
                    <circle class="hv-src" cx="10" cy="22" r="4" /><circle class="hv-src" cx="10" cy="60" r="4" /><circle class="hv-src" cx="10" cy="98" r="4" />
                  </svg>
                  <svg v-else-if="capability.key === 'routing'" viewBox="0 0 260 120">
                    <path class="hv-line is-dim" d="M30 60 C 110 60, 140 20, 220 20" />
                    <path class="hv-flow" d="M30 60 C 110 60, 140 60, 220 60" />
                    <path class="hv-line" d="M30 60 C 110 60, 140 100, 220 100" />
                    <circle class="hv-node" cx="30" cy="60" r="10" /><circle class="hv-core" cx="30" cy="60" r="4" />
                    <circle class="hv-warn" cx="220" cy="20" r="7" /><circle class="hv-ok" cx="220" cy="60" r="7" /><circle class="hv-src" cx="220" cy="100" r="6" />
                  </svg>
                  <div v-else-if="capability.key === 'keys'" class="hv-keys">
                    <div class="hv-key"><span class="mono">sk-••••••8f2a</span><i /></div>
                    <div v-for="row in keyRows" :key="row" class="hv-toggle"><span>{{ row }}</span><b /></div>
                  </div>
                  <div v-else-if="capability.key === 'usage'" class="hv-bars">
                    <i v-for="(height, index) in usageBars" :key="index" :style="{ '--h': `${height}%`, '--n': index + 1 }" />
                  </div>
                  <div v-else-if="capability.key === 'stream'" class="hv-stream">
                    <span v-for="row in 4" :key="row" :style="{ '--n': row }"><i /><i /><i /></span>
                  </div>
                  <div v-else class="hv-balance">
                    <svg viewBox="0 0 120 120"><circle class="hv-ring" cx="60" cy="60" r="48" /><circle class="hv-ring-fill" cx="60" cy="60" r="48" /></svg>
                    <div class="hv-balance-value"><strong>¥ 240</strong><span class="mono">{{ t('home.orbit.panelQuota') }}</span></div>
                  </div>
                </div>
                <h3 class="home-h3">{{ capability.title }}</h3>
                <p class="home-body">{{ capability.description }}</p>
              </article>
            </div>
          </div>
        </div>
      </section>

      <section id="warp" class="home-chapter warp" data-chapter data-pin>
        <div class="home-pin warp-pin">
          <div class="home-container warp-inner">
            <p class="eyebrow" data-reveal>{{ t('home.orbit.warpLabel') }}</p>
            <h2 class="warp-title">
              <span data-reveal="left">{{ t('home.orbit.warpTitleA') }}</span>
              <span class="hl" data-reveal="right">{{ t('home.orbit.warpTitleB') }}</span>
            </h2>
            <p class="home-lead" data-reveal>{{ t('home.orbit.warpLead') }}</p>
            <div class="warp-tags">
              <span v-for="tag in warpTags" :key="tag" class="chip"><i />{{ tag }}</span>
            </div>
          </div>
        </div>
      </section>

      <section id="control" class="home-chapter control" data-chapter data-scene>
        <div class="home-container split is-reversed">
          <div class="stage" aria-hidden="true">
            <div class="console">
              <div class="home-card panel" data-depth="0.6">
                <div class="panel-head"><span>{{ t('home.orbit.panelTitle') }}</span><span class="num">{{ t('home.orbit.panelProject') }}</span></div>
                <div class="panel-metric"><strong>{{ t('home.orbit.panelToday') }}</strong><span class="pill">{{ t('home.orbit.panelActive') }}</span></div>
                <div class="panel-chart">
                  <i v-for="index in 14" :key="index" :style="{ '--h': `${18 + (index * 37) % 74}%`, '--n': index }" />
                </div>
                <div class="panel-quota"><span>{{ t('home.orbit.panelQuota') }}</span><i><b /></i><span class="mono">62%</span></div>
              </div>
              <div class="home-card float-card float-key" data-depth="1.6" data-speed="-0.06">
                <span class="num">sk-••••••8f2a</span><span class="badge">{{ t('home.orbit.panelActive') }}</span>
              </div>
              <div class="home-card float-card float-ring" data-depth="2.2" data-speed="-0.12">
                <svg viewBox="0 0 120 120"><circle class="hv-ring" cx="60" cy="60" r="44" /><circle class="hv-ring-fill" cx="60" cy="60" r="44" /></svg>
              </div>
              <div class="home-card float-card float-log" data-depth="1.2" data-speed="0.05">
                <span class="num">{{ t('home.orbit.panelLogs') }}</span>
                <span class="row mono">200 OK · 1.2s</span>
                <span class="row mono">200 OK · 0.8s</span>
                <span class="row mono is-active">200 OK · 0.4s</span>
              </div>
            </div>
          </div>
          <div class="control-copy">
            <p class="eyebrow" data-reveal><b>03</b>{{ t('home.orbit.controlLabel') }}</p>
            <SplitText tag="h2" class="home-h2" :text="t('home.orbit.controlTitle')" data-reveal />
            <p class="home-lead" data-reveal style="--d: 180ms">{{ t('home.orbit.controlDescription') }}</p>
            <ul class="bullets">
              <li v-for="index in 4" :key="index" data-reveal><span class="num">{{ String(index).padStart(2, '0') }}</span>{{ t(`home.orbit.bullet${index}`) }}</li>
            </ul>
          </div>
        </div>
      </section>

      <section id="clients" class="home-chapter clients" data-chapter>
        <div class="clients-inner">
          <div class="home-container clients-head">
            <p class="eyebrow" data-reveal>{{ t('home.orbit.clientsLabel') }}</p>
            <SplitText tag="h2" class="home-h2" :text="t('home.orbit.clientsTitle')" data-reveal />
            <p class="home-lead" data-reveal style="--d: 180ms">{{ t('home.orbit.clientsDescription') }}</p>
          </div>
          <div class="marquees" data-reveal="fade" style="--d: 260ms">
            <div class="marquee">
              <div class="marquee-track" style="--dur: 46s">
                <ul><li v-for="name in clientsA" :key="name" class="marquee-item"><i />{{ name }}</li></ul>
                <ul aria-hidden="true"><li v-for="name in clientsA" :key="`d-${name}`" class="marquee-item"><i />{{ name }}</li></ul>
              </div>
            </div>
            <div class="marquee is-reverse">
              <div class="marquee-track" style="--dur: 54s">
                <ul><li v-for="name in clientsB" :key="name" class="marquee-item"><i />{{ name }}</li></ul>
                <ul aria-hidden="true"><li v-for="name in clientsB" :key="`d-${name}`" class="marquee-item"><i />{{ name }}</li></ul>
              </div>
            </div>
          </div>
          <p class="clients-note">{{ t('home.orbit.clientsNote') }}</p>
          <div class="integration-paths" data-reveal="fade" style="--d: 360ms">
            <div class="integration-paths-head">
              <span class="num">{{ t('home.orbit.integrationLabel') }}</span>
              <p>{{ t('home.orbit.integrationDescription') }}</p>
            </div>
            <ul>
              <li v-for="path in integrationPaths" :key="path.key">
                <span class="integration-path-index">{{ path.index }}</span>
                <div>
                  <strong>{{ path.title }}</strong>
                  <span>{{ path.description }}</span>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </section>

      <section id="start" class="home-chapter steps" data-chapter data-scene>
        <div class="home-container steps-grid">
          <div>
            <p class="eyebrow" data-reveal><b>04</b>{{ t('home.orbit.startLabel') }}</p>
            <SplitText tag="h2" class="home-h2" :text="t('home.orbit.startTitle')" data-reveal />
            <router-link v-magnetic class="home-btn home-btn-primary" :to="primaryDestination" data-reveal style="--d: 200ms">
              {{ primaryLabel }}<i aria-hidden="true">→</i>
            </router-link>
          </div>
          <ol class="steps-list">
            <li v-for="step in 3" :key="step" data-reveal :style="{ '--d': `${step * 120}ms` }">
              <span class="num">0{{ step }}</span>
              <div>
                <h3 class="home-h3">{{ t(`home.orbit.step${step}Title`) }}</h3>
                <p class="home-body">{{ t(`home.orbit.step${step}Description`) }}</p>
              </div>
            </li>
          </ol>
        </div>
      </section>

      <section id="faq" class="home-chapter faq" data-chapter>
        <div class="home-container faq-grid">
          <div>
            <p class="eyebrow" data-reveal><b>05</b>{{ t('home.orbit.faqLabel') }}</p>
            <h2 class="home-h2" data-reveal>{{ t('home.orbit.faqTitle') }}</h2>
            <p class="home-lead" data-reveal style="--d: 160ms">{{ t('home.orbit.faqDescription') }}</p>
          </div>
          <div class="faq-list">
            <details v-for="question in 4" :key="question" data-reveal :style="{ '--d': `${question * 70}ms` }">
              <summary><span class="num">{{ String(question).padStart(2, '0') }}</span><b>{{ t(`home.orbit.question${question}`) }}</b><span class="faq-plus" aria-hidden="true" /></summary>
              <p class="home-body">
                {{ question === 1 && !registrationEnabled ? t('home.orbit.registrationClosed') : t(`home.orbit.answer${question}`) }}
              </p>
            </details>
          </div>
        </div>
      </section>

      <section id="finale" class="home-chapter finale" data-chapter>
        <div class="home-container">
          <SplitText tag="h2" class="home-display" :text="t('home.orbit.finaleTitle')" data-reveal />
          <p class="home-lead" data-reveal style="--d: 260ms">{{ t('home.orbit.finaleLead') }}</p>
          <div class="btn-row" data-reveal style="--d: 420ms">
            <router-link v-magnetic class="home-btn home-btn-primary" :to="primaryDestination">{{ primaryLabel }}<i aria-hidden="true">→</i></router-link>
            <a v-if="docUrl" v-magnetic class="home-btn home-btn-ghost" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.viewDocs') }}</a>
          </div>
        </div>
      </section>
    </main>

    <footer class="home-footer">
      <div class="home-footer-glow" aria-hidden="true" />
      <div class="home-container home-footer-top">
        <div class="footer-brand">
          <router-link to="/home" class="home-brand">
            <img v-if="siteLogo" :src="siteLogo" alt="" class="home-brand-img" />
            <svg v-else class="home-brand-mark" viewBox="0 0 96 96" fill="none" aria-hidden="true">
              <rect width="96" height="96" rx="26" class="mark-tile" />
              <circle cx="48" cy="48" r="18" stroke-width="7" class="mark-core" />
              <g transform="rotate(-35 48 48)"><ellipse cx="48" cy="48" rx="36" ry="14" stroke-width="3" class="mark-orbit" /></g>
              <circle cx="76" cy="29" r="5" class="mark-node" />
            </svg>
            <span class="home-brand-text"><strong>{{ siteName }}</strong><small>{{ t('home.orbit.kicker') }}</small></span>
          </router-link>
          <p class="footer-lead">{{ siteSubtitle }}</p>
        </div>
        <nav class="footer-cols">
          <div>
            <h3>{{ t('home.orbit.chapters') }}</h3>
            <a href="#connect">{{ t('home.orbit.platform') }}</a>
            <a href="#capabilities">{{ t('home.orbit.chapterCaps') }}</a>
            <a href="#start">{{ t('home.orbit.start') }}</a>
            <a href="#faq">{{ t('home.orbit.faq') }}</a>
          </div>
          <div>
            <h3>{{ t('home.viewDocs') }}</h3>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }} ↗</a>
            <router-link to="/desktop">{{ t('home.desktopDownload') }}</router-link>
            <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub</a>
          </div>
          <div>
            <h3>{{ t('home.orbit.start') }}</h3>
            <router-link v-if="isAuthenticated" :to="dashboardPath">{{ t('home.dashboard') }}</router-link>
            <template v-else>
              <router-link to="/login">{{ t('home.login') }}</router-link>
              <router-link v-if="registrationEnabled" to="/register">{{ t('home.getStarted') }}</router-link>
            </template>
          </div>
        </nav>
      </div>
      <div class="home-footer-word" aria-hidden="true"><span data-speed="-0.12">{{ wordmark }}</span></div>
      <div class="home-container home-footer-bottom">
        <p>&copy; {{ currentYear }} {{ siteName }}</p>
        <button type="button" class="top-btn" @click="scrollToChapter(0)">{{ t('home.orbit.backToTop') }}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 19V5M6 11l6-6 6 6" /></svg></button>
      </div>
    </footer>

    <button v-if="!reducedMotion" class="orbit-motion" type="button" :aria-pressed="motionPaused" @click="motionPaused = !motionPaused">
      <Icon v-if="motionPaused" name="play" size="sm" />
      <span v-else aria-hidden="true">Ⅱ</span>
      {{ motionPaused ? t('home.orbit.resume') : t('home.orbit.pause') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref, watch } from 'vue'
import { useDark, useToggle, useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import SplitText from '@/components/home/SplitText.vue'
import CursorAura from '@/components/home/CursorAura.vue'
import { vMagnetic, vTilt } from '@/components/home/directives'
import { useHomeMotion } from '@/components/home/useHomeMotion'
import { splitTokens } from '@/components/home/splitTokens'
import type { Chapter } from '@/components/home/universe'
import { sanitizeUrl } from '@/utils/url'
import { buildGatewayUrl } from '@/api/url'
import '@fontsource-variable/manrope'

const OrbitScene = defineAsyncComponent(() => import('@/components/home/OrbitScene.vue'))
const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const siteName = computed(() => {
  const name = appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API'
  return name.toLowerCase() === 'sub2api' ? '小羊咩咩' : name
})
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || t('home.heroSubtitle'))
const customSubtitle = computed(() => siteSubtitle.value.trim())
const wordmark = computed(() => appStore.cachedPublicSettings?.home_particle_text?.trim() || siteName.value)
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const isHomeContentUrl = computed(() => /^https?:\/\//.test(homeContent.value.trim()))
const isDark = useDark({ storageKey: 'theme' })
const toggleTheme = useToggle(isDark)
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const registrationEnabled = computed(() => appStore.cachedPublicSettings?.registration_enabled === true)
const primaryDestination = computed(() => isAuthenticated.value ? dashboardPath.value : registrationEnabled.value ? '/register' : '/login')
const primaryLabel = computed(() => t(isAuthenticated.value ? 'home.goToDashboard' : registrationEnabled.value ? 'home.getStarted' : 'home.login'))
const currentYear = new Date().getFullYear()
const motionPaused = ref(false)
const menuOpen = ref(false)
const titleVisible = ref(false)
const reducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
const lowMotion = computed(() => motionPaused.value || reducedMotion.value)
const host = ref<HTMLElement>()
const progressBar = ref<HTMLElement>()
const narrow = useMediaQuery('(max-width: 899px)')
const { activeChapter, scrolled, scrollToChapter } = useHomeMotion(host, lowMotion, progressBar)

const baseUrl = computed(() => {
  const configured = sanitizeUrl(appStore.cachedPublicSettings?.api_base_url || '')
  const base = (configured || buildGatewayUrl('/v1')).replace(/\/+$/, '')
  return base.endsWith('/v1') ? base : `${base}/v1`
})
const copied = ref(false)
const copyError = ref(false)
let copyTimer: number | undefined
async function copyEndpoint() {
  try {
    await navigator.clipboard.writeText(baseUrl.value)
    copied.value = true
    copyError.value = false
  } catch {
    copyError.value = true
  }
  window.clearTimeout(copyTimer)
  copyTimer = window.setTimeout(() => { copied.value = false; copyError.value = false }, 2400)
}

// Particle formations and camera moves, one per chapter of the page.
const chapters = computed<Chapter[]>(() => [
  { formation: 'core', title: 1, camera: { dist: 8.7, pitch: 0.16, ty: 0.05, sy: 0.08, drift: 0.12 }, narrow: { sy: 0.1 } },
  { formation: 'core', fade: 0.5, camera: { dist: 5.4, pitch: 0.52, yaw: 0.62, ty: -0.2, drift: 0.32 } },
  { formation: 'sphere', fade: 0.95, camera: { dist: 12.6, yaw: -0.2, pitch: 0.14, sx: 0.5, drift: 0.34 }, narrow: { sy: 0 }, narrowFade: 0.5 },
  { formation: 'rings', fade: 0.55, camera: { dist: 10.6, yaw: 0.25, pitch: 0.3, sy: 0.12, drift: 0.7 } },
  { formation: 'tunnel', camera: { dist: 8, pitch: 0, yaw: 0, tz: -4, fov: 1.05, drift: 0 } },
  { formation: 'helix', fade: 0.62, camera: { dist: 11.5, pitch: 0.06, yaw: -0.35, sy: 0.16, drift: 0.18 }, narrow: { sy: 0.28 }, narrowFade: 0.4 },
  { formation: 'halo', fade: 0.62, camera: { dist: 11.5, pitch: 0.04, sy: 0.3, drift: 0.12 }, narrow: { sy: 0.36 } },
  { formation: 'tunnel', fade: 0.5, camera: { dist: 12, pitch: 0.28, yaw: 0.5, ty: -0.5, tz: -2, drift: 0.22 } },
  { formation: 'halo', fade: 0.34, camera: { dist: 16, pitch: 0.12, yaw: -0.4, sx: -0.32, drift: 0.2 } },
  { formation: 'wordmark', fade: 0.9, camera: { dist: 10.2, pitch: 0.03, sy: 0.3, drift: 0.05 }, narrow: { sy: 0.34, dist: 15.5 } }
])
const railLabels = computed(() => ['chapterHero', 'chapterManifesto', 'chapterConnect', 'chapterCaps', 'chapterWarp', 'chapterControl', 'chapterClients', 'chapterStart', 'chapterFaq', 'chapterFinale'].map(key => t(`home.orbit.${key}`)))

const manifestoTokens = computed(() => splitTokens(t('home.orbit.manifesto')))
const manifestoTotal = computed(() => manifestoTokens.value.filter(token => !token.space).length)

const capabilities = computed(() => [
  { key: 'access', title: t('home.orbit.capAccess'), description: t('home.orbit.capAccessDesc') },
  { key: 'routing', title: t('home.orbit.capRouting'), description: t('home.orbit.capRoutingDesc') },
  { key: 'keys', title: t('home.orbit.capKeys'), description: t('home.orbit.capKeysDesc') },
  { key: 'usage', title: t('home.orbit.capUsage'), description: t('home.orbit.capUsageDesc') },
  { key: 'stream', title: t('home.orbit.capStream'), description: t('home.orbit.capStreamDesc') },
  { key: 'balance', title: t('home.orbit.capBalance'), description: t('home.orbit.capBalanceDesc') }
])
const keyRows = computed(() => [t('home.orbit.keyQuota'), t('home.orbit.keyExpiry'), t('home.orbit.keyAllowlist')])
const usageBars = [38, 52, 44, 66, 58, 74, 62, 84, 70, 92, 78, 88]
const warpTags = ['SSE', 'WebSocket', 'Chat Completions', 'Responses', 'Messages']
const constellation = [
  { name: 'Claude', x: 8, y: 16, depth: 1.4, delay: 0 },
  { name: 'GPT-5', x: 62, y: 4, depth: 0.8, delay: 1.1 },
  { name: 'Gemini', x: 24, y: 62, depth: 2.2, delay: 2.3 },
  { name: 'DeepSeek', x: 70, y: 48, depth: 1.8, delay: 0.6 },
  { name: 'Qwen', x: 46, y: 82, depth: 1.1, delay: 1.7 }
]
const clientsA = ['OpenAI SDK', 'Anthropic SDK', 'Vercel AI SDK', 'LangChain', 'LlamaIndex', 'Dify', 'n8n']
const clientsB = ['Cherry Studio', 'NextChat', 'LobeChat', 'Chatbox', 'Cline', 'Continue', 'SillyTavern', 'Open WebUI']
const integrationPaths = computed(() => [
  { key: 'sdk', index: '01', title: t('home.orbit.integrationSdk'), description: t('home.orbit.integrationSdkDesc') },
  { key: 'cli', index: '02', title: t('home.orbit.integrationCli'), description: t('home.orbit.integrationCliDesc') },
  { key: 'workflow', index: '03', title: t('home.orbit.integrationWorkflow'), description: t('home.orbit.integrationWorkflowDesc') },
  { key: 'custom', index: '04', title: t('home.orbit.integrationCustom'), description: t('home.orbit.integrationCustomDesc') },
])

// Horizontal travel for the pinned capability track.
const capsSection = ref<HTMLElement>()
const capsTrack = ref<HTMLElement>()
const capsShift = ref(0)
const measureCaps = () => {
  const track = capsTrack.value
  const section = capsSection.value
  if (!track || !section) return
  capsShift.value = Math.max(0, Math.round(track.scrollWidth - section.clientWidth))
}
onMounted(() => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
  measureCaps()
  window.addEventListener('resize', measureCaps, { passive: true })
  void document.fonts?.ready.then(measureCaps).catch(() => {})
})
watch(capsSection, measureCaps)
</script>

<style scoped>
.home-site {
  --home-bg: #050d17; --home-bg-2: #091725;
  --home-ink: #f2f7fa; --home-ink-2: rgba(222, 238, 246, .74); --home-ink-3: rgba(190, 218, 232, .5);
  --home-line: rgba(150, 220, 240, .12); --home-line-2: rgba(150, 220, 240, .24);
  --home-accent: #70e0ed; --home-accent-2: #2ec4da; --home-accent-soft: rgba(112, 224, 237, .1);
  --home-amber: #f3c77b;
  --home-surface: rgba(10, 27, 42, .66); --home-surface-2: rgba(15, 38, 57, .82);
  --home-btn-bg: #70e0ed; --home-btn-ink: #04212b;
  --home-header-bg: rgba(5, 14, 24, .84);
  --home-halo: radial-gradient(closest-side, rgba(3, 9, 16, .72), transparent);
  --universe-bg: radial-gradient(90% 70% at 50% 45%, rgba(14, 44, 66, .55), transparent 70%), linear-gradient(180deg, #060f1a, #040a12);
  --nebula-a: radial-gradient(38% 30% at 72% 30%, rgba(38, 187, 209, .16), transparent 70%), radial-gradient(34% 28% at 22% 72%, rgba(36, 92, 168, .18), transparent 72%);
  --nebula-b: radial-gradient(26% 22% at 84% 82%, rgba(243, 199, 123, .07), transparent 70%);
  --star-a: rgba(255, 255, 255, .8); --star-b: rgba(200, 245, 252, .7); --star-c: rgba(112, 224, 237, .8); --star-d: rgba(243, 199, 123, .7);
  --home-ease: cubic-bezier(.16, 1, .3, 1);
  --home-font: 'Manrope Variable', 'Manrope', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', system-ui, sans-serif;
  --home-mono: ui-monospace, 'SF Mono', 'JetBrains Mono', 'Cascadia Code', Menlo, Consolas, monospace;
  position: relative;
  min-height: 100vh;
  color: var(--home-ink);
  background: var(--home-bg);
  font-family: var(--home-font);
  font-feature-settings: 'ss01', 'cv11';
  -webkit-font-smoothing: antialiased;
  overflow-x: clip;
}
  .home-site.is-light {
    --home-bg: #eef3f6; --home-bg-2: #e2ebf1;
    --home-ink: #0a1a22; --home-ink-2: rgba(16, 44, 56, .78); --home-ink-3: rgba(20, 55, 70, .54);
    --home-line: rgba(12, 70, 88, .16); --home-line-2: rgba(12, 70, 88, .3);
    --home-accent: #0b7c8c; --home-accent-2: #0f98ad; --home-accent-soft: rgba(11, 124, 140, .1);
    --home-amber: #b57a12;
    --home-surface: rgba(255, 255, 255, .74); --home-surface-2: rgba(255, 255, 255, .92);
    --home-btn-bg: #0b7c8c; --home-btn-ink: #f2fbfd;
    --home-header-bg: rgba(243, 249, 251, .86);
    --home-halo: radial-gradient(closest-side, rgba(255, 255, 255, .78), transparent);
    --universe-bg: radial-gradient(90% 70% at 50% 45%, rgba(178, 226, 240, .6), transparent 72%), linear-gradient(180deg, #f2f8fa, #e6eef3);
    --nebula-a: radial-gradient(38% 30% at 72% 30%, rgba(46, 196, 218, .16), transparent 70%), radial-gradient(34% 28% at 22% 72%, rgba(90, 150, 220, .14), transparent 72%);
    --nebula-b: radial-gradient(26% 22% at 84% 82%, rgba(201, 138, 28, .08), transparent 70%);
    --star-a: rgba(12, 62, 82, .5); --star-b: rgba(20, 90, 110, .42); --star-c: rgba(11, 124, 140, .5); --star-d: rgba(181, 122, 18, .45);
}
.home-site ::selection { background: color-mix(in srgb, var(--home-accent) 32%, transparent); }
.home-site :where(a) { color: inherit; text-decoration: none; }
.home-site :focus-visible { outline: 2px solid var(--home-accent); outline-offset: 3px; border-radius: 10px; }
.home-site :where(h1, h2, h3, p, ul, ol, li) { margin: 0; padding: 0; }
.home-site ul, .home-site ol { list-style: none; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); clip-path: inset(50%); white-space: nowrap; border: 0; }
.home-main { position: relative; z-index: 2; }
.home-vignette { position: fixed; inset: 0; z-index: 1; pointer-events: none; background: radial-gradient(120% 90% at 50% 40%, transparent 55%, rgba(1, 4, 9, .62) 100%); }
.is-light .home-vignette { opacity: .12; }
.home-skip { position: fixed; left: 16px; top: 12px; z-index: 100; padding: 10px 16px; border-radius: 12px; background: var(--home-btn-bg); color: var(--home-btn-ink); font-weight: 700; font-size: 14px; transform: translateY(-160%); transition: transform .25s; }
.home-skip:focus { transform: none; }
.home-progress { position: fixed; left: 0; right: 0; top: 0; z-index: 60; height: 2px; pointer-events: none; }
.home-progress span { display: block; height: 100%; transform: scaleX(0); transform-origin: 0 50%; background: linear-gradient(90deg, var(--home-accent-2), var(--home-accent) 70%, var(--home-amber)); }

/* Header */
.home-header { position: fixed; left: 0; right: 0; top: 0; z-index: 50; padding: 16px clamp(12px, 2.4vw, 36px) 0; transition: padding .45s var(--home-ease); }
.home-header.is-compact { padding-top: 10px; }
.home-header-bar { position: relative; display: flex; align-items: center; gap: 18px; height: 66px; padding: 0 10px 0 12px; border-radius: 24px; border: 1px solid transparent; transition: background .45s, border-color .45s, box-shadow .45s, height .45s var(--home-ease); }
.home-header.is-compact .home-header-bar { height: 58px; background: var(--home-header-bg); border-color: var(--home-line); box-shadow: 0 24px 60px -36px rgba(0, 0, 0, .55); backdrop-filter: blur(14px); }
.home-brand { display: inline-flex; align-items: center; gap: 12px; flex-shrink: 0; }
.home-brand-mark { width: 38px; height: 38px; }
.home-brand-img { width: 38px; height: 38px; border-radius: 11px; object-fit: contain; }
.mark-tile { fill: color-mix(in srgb, var(--home-accent) 14%, var(--home-bg-2)); }
.mark-core { stroke: var(--home-accent); fill: none; }
.mark-orbit { stroke: var(--home-accent-2); fill: none; }
.mark-node { fill: var(--home-amber); }
.home-brand-text { display: flex; flex-direction: column; line-height: 1; min-width: 0; }
.home-brand-text strong { font-size: 17px; font-weight: 750; letter-spacing: -.03em; max-width: 190px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.home-brand-text small { margin-top: 5px; font-size: 8.5px; font-weight: 700; letter-spacing: .24em; text-transform: uppercase; color: var(--home-ink-3); }
.home-nav { margin: 0 auto; display: flex; align-items: center; gap: 2px; padding: 5px; border-radius: 999px; border: 1px solid var(--home-line); background: color-mix(in srgb, var(--home-surface) 70%, transparent); }
.home-nav-link { display: inline-flex; align-items: center; padding: 9px 16px; border-radius: 999px; font-size: 13.5px; font-weight: 600; color: var(--home-ink-2); white-space: nowrap; transition: color .25s, background .25s; }
.home-nav-link:hover, .home-nav-link.is-active { color: var(--home-ink); background: var(--home-accent-soft); }
.home-actions { display: flex; align-items: center; gap: 6px; margin-left: auto; }
.home-nav + .home-actions { margin-left: 0; }
.home-actions :deep(button) { border-radius: 999px; }
.home-icon { display: inline-flex; align-items: center; justify-content: center; width: 40px; height: 40px; border-radius: 999px; border: 0; background: transparent; color: var(--home-ink-2); cursor: pointer; }
.home-icon:hover { color: var(--home-ink); background: var(--home-accent-soft); }
.home-login { padding: 9px 14px; border-radius: 999px; font-size: 13.5px; font-weight: 650; color: var(--home-ink-2); }
.home-login:hover { color: var(--home-ink); }
.home-cta { display: inline-flex; align-items: center; gap: 8px; height: 42px; padding: 0 8px 0 18px; border-radius: 999px; background: var(--home-btn-bg); color: var(--home-btn-ink); font-size: 13.5px; font-weight: 700; white-space: nowrap; box-shadow: 0 10px 30px -14px var(--home-accent); transform: translate3d(var(--mx, 0), var(--my, 0), 0); transition: transform .5s var(--home-ease), box-shadow .3s; }
.home-cta.is-magnet { transition: transform .16s linear, box-shadow .3s; }
.home-cta:hover { box-shadow: 0 16px 40px -12px var(--home-accent); }
.home-cta i { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 50%; font-style: normal; font-size: 15px; background: color-mix(in srgb, var(--home-btn-ink) 12%, transparent); transition: transform .35s var(--home-ease); }
.home-cta:hover i { transform: rotate(45deg); }
.home-menu-btn { display: none; position: relative; width: 42px; height: 42px; border-radius: 999px; border: 1px solid var(--home-line-2); background: transparent; cursor: pointer; }
.home-menu-btn span { position: absolute; left: 12px; right: 12px; height: 1.6px; border-radius: 2px; background: var(--home-ink); transition: transform .4s var(--home-ease), top .4s var(--home-ease); }
.home-menu-btn span:first-child { top: 16px; }
.home-menu-btn span:last-child { top: 24px; }
.home-menu { position: absolute; left: clamp(12px, 2.4vw, 36px); right: clamp(12px, 2.4vw, 36px); top: calc(100% + 8px); display: grid; padding: 14px; border-radius: 20px; border: 1px solid var(--home-line); background: var(--home-header-bg); backdrop-filter: blur(16px); }
.home-menu nav { display: grid; }
.home-menu a { padding: 12px 14px; border-radius: 12px; font-size: 14px; font-weight: 600; color: var(--home-ink-2); }
.home-menu a:hover { color: var(--home-ink); background: var(--home-accent-soft); }

/* Rail */
.home-rail { position: fixed; right: clamp(14px, 1.8vw, 30px); top: 50%; z-index: 40; display: flex; flex-direction: column; gap: 6px; transform: translateY(-50%); }
.home-rail-dot { position: relative; display: flex; align-items: center; justify-content: flex-end; gap: 12px; height: 22px; padding: 0; border: 0; background: transparent; cursor: pointer; color: var(--home-ink-3); border-radius: 999px; }
.home-rail-dot i { width: 7px; height: 7px; border-radius: 50%; border: 1px solid var(--home-line-2); transition: transform .4s var(--home-ease), background .3s, border-color .3s, box-shadow .3s; }
.home-rail-label { padding: 4px 10px; border-radius: 999px; border: 1px solid var(--home-line); background: color-mix(in srgb, var(--home-bg) 88%, transparent); font-size: 10.5px; font-weight: 700; letter-spacing: .16em; text-transform: uppercase; white-space: nowrap; opacity: 0; transform: translateX(6px); transition: opacity .3s, transform .4s var(--home-ease); pointer-events: none; }
.home-rail:hover .home-rail-label, .home-rail:focus-within .home-rail-label { opacity: .75; transform: none; }
.home-rail-dot:hover .home-rail-label, .home-rail-dot.is-active .home-rail-label { opacity: 1; }
.home-rail-dot.is-active { color: var(--home-ink); }
.home-rail-dot.is-active i { transform: scale(1.5); background: var(--home-accent); border-color: var(--home-accent); box-shadow: 0 0 14px var(--home-accent); }

/* Chapter scaffolding */
.home-container { position: relative; width: min(1240px, calc(100% - clamp(36px, 7vw, 120px))); margin: 0 auto; }
.home-chapter { position: relative; display: flex; align-items: center; min-height: 100vh; min-height: 100svh; padding: clamp(110px, 14vh, 170px) 0 clamp(80px, 11vh, 130px); }
.home-pin { position: sticky; top: 0; display: flex; align-items: center; width: 100%; height: 100vh; height: 100svh; overflow: hidden; }
.home-chapter[data-pin] { display: block; }
.eyebrow { display: inline-flex; align-items: center; gap: 12px; font-size: 11.5px; font-weight: 750; letter-spacing: .28em; text-transform: uppercase; color: var(--home-accent); }
.eyebrow::before { content: ''; width: 28px; height: 1px; background: currentColor; opacity: .7; }
.eyebrow b { color: var(--home-ink-3); font-weight: 750; }
.home-display { font-size: clamp(46px, 7.4vw, 116px); font-weight: 720; line-height: 1.06; letter-spacing: -.048em; }
.home-h2 { font-size: clamp(36px, 5.2vw, 76px); font-weight: 700; line-height: 1.1; letter-spacing: -.042em; }
.home-h3 { font-size: clamp(20px, 1.9vw, 27px); font-weight: 700; line-height: 1.3; letter-spacing: -.02em; }
.home-lead { font-size: clamp(16px, 1.22vw, 19px); line-height: 1.78; color: var(--home-ink-2); max-width: 34em; }
.home-body { font-size: 15px; line-height: 1.78; color: var(--home-ink-2); }
:lang(zh) .home-display { letter-spacing: -.02em; line-height: 1.16; }
:lang(zh) .home-h2 { letter-spacing: -.015em; line-height: 1.2; }
:lang(zh) .home-lead, :lang(zh) .home-body { line-height: 1.9; }
.num, .mono { font-family: var(--home-mono); }
.num { font-size: 12px; letter-spacing: .14em; color: var(--home-ink-3); }
.hl { color: var(--home-accent); }
.split-text, :deep(.split-inner), :deep(.split-line) { display: block; }
:deep(.split-ch) { display: inline-block; white-space: pre; }
:deep(.split-ch.is-hl) { color: var(--home-accent); text-shadow: 0 0 34px color-mix(in srgb, var(--home-accent) 45%, transparent); }
.home-site [data-reveal] { opacity: 0; transform: translate3d(0, 36px, 0); transition: opacity 1s var(--home-ease) var(--d, 0ms), transform 1.2s var(--home-ease) var(--d, 0ms); }
.home-site [data-reveal='fade'] { transform: none; }
.home-site [data-reveal='scale'] { transform: scale(.94); }
.home-site [data-reveal='left'] { transform: translate3d(-48px, 0, 0); }
.home-site [data-reveal='right'] { transform: translate3d(48px, 0, 0); }
.home-site [data-reveal].is-in { opacity: 1; transform: none; }
.home-site .split-text[data-reveal] { opacity: 1; transform: none; transition: none; }
.split-text[data-reveal] :deep(.split-ch) { opacity: 0; transform: translate3d(0, .5em, 0) rotateX(-62deg); transform-origin: 50% 100%; transition: opacity .8s ease calc(var(--d, 0ms) + var(--i, 0) * 32ms), transform 1.1s var(--home-ease) calc(var(--d, 0ms) + var(--i, 0) * 32ms); }
.split-text[data-reveal].is-in :deep(.split-ch) { opacity: 1; transform: none; }
:deep(.split-line) { perspective: 800px; }

/* Buttons, chips, cards */
.home-btn { position: relative; display: inline-flex; align-items: center; justify-content: center; gap: 14px; height: 56px; padding: 0 26px; border-radius: 999px; border: 1px solid transparent; font: 700 15px/1 var(--home-font); letter-spacing: -.005em; white-space: nowrap; cursor: pointer; transform: translate3d(var(--mx, 0), var(--my, 0), 0); transition: transform .55s var(--home-ease), background .3s, box-shadow .35s, border-color .3s, color .3s; }
.home-btn.is-magnet { transition: transform .16s linear, background .3s, box-shadow .35s, border-color .3s, color .3s; }
.home-btn-primary { padding: 0 9px 0 28px; background: var(--home-btn-bg); color: var(--home-btn-ink); box-shadow: 0 0 0 1px color-mix(in srgb, var(--home-accent) 40%, transparent), 0 24px 60px -22px var(--home-accent); }
.home-btn-primary i { display: grid; place-items: center; width: 40px; height: 40px; border-radius: 50%; background: var(--home-btn-ink); color: var(--home-btn-bg); font-style: normal; font-size: 18px; transition: transform .5s var(--home-ease); }
.home-btn-primary:hover i { transform: translate(2px) rotate(-45deg); }
.home-btn-ghost { border-color: var(--home-line-2); background: color-mix(in srgb, var(--home-surface) 60%, transparent); color: var(--home-ink); }
.home-btn-ghost:hover { border-color: var(--home-accent); }
.btn-row { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
.chips { display: flex; flex-wrap: wrap; gap: 8px; }
.chip { display: inline-flex; align-items: center; gap: 8px; height: 34px; padding: 0 14px; border-radius: 999px; border: 1px solid var(--home-line); background: var(--home-surface); font-size: 13px; font-weight: 650; color: var(--home-ink-2); white-space: nowrap; }
.chip i { width: 6px; height: 6px; border-radius: 50%; background: var(--home-accent); box-shadow: 0 0 10px var(--home-accent); }
.chip i.is-amber { background: var(--home-amber); box-shadow: 0 0 10px var(--home-amber); }
.home-card { position: relative; isolation: isolate; border-radius: 26px; border: 1px solid var(--home-line); background: linear-gradient(180deg, var(--home-surface-2), var(--home-surface)); box-shadow: inset 0 1px rgba(255, 255, 255, .06), 0 40px 90px -50px rgba(0, 0, 0, .9); overflow: hidden; }
.home-card::before { content: ''; position: absolute; inset: 0; z-index: -1; pointer-events: none; background: radial-gradient(520px circle at var(--hx, 50%) var(--hy, 0%), color-mix(in srgb, var(--home-accent) 16%, transparent), transparent 62%); opacity: 0; transition: opacity .45s; }
.home-card.is-tilting::before { opacity: 1; }
.home-card.is-tilting { border-color: var(--home-line-2); }

/* Hero */
.hero { padding: 0; align-items: stretch; }
.hero-inner { display: flex; flex-direction: column; justify-content: space-between; min-height: 100vh; min-height: 100svh; padding: 118px 0 clamp(42px, 7vh, 72px); }
.hero-kicker { display: inline-flex; align-items: center; gap: 16px; margin: clamp(6px, 4vh, 40px) auto 0; font-size: 11px; font-weight: 750; letter-spacing: .42em; text-transform: uppercase; color: var(--home-ink-2); }
.hero-kicker i { width: 46px; height: 1px; background: linear-gradient(90deg, transparent, var(--home-accent)); }
.hero-kicker i:last-child { transform: scaleX(-1); }
.hero-wordmark { position: absolute; left: 0; right: 0; top: 46%; transform: translateY(-50%); text-align: center; pointer-events: none; }
.hero-word { display: inline-block; font-size: clamp(76px, 15.2vw, 236px); font-weight: 800; letter-spacing: -.045em; line-height: .9; background: linear-gradient(180deg, var(--home-ink) 30%, color-mix(in srgb, var(--home-accent) 70%, var(--home-ink))); -webkit-background-clip: text; background-clip: text; color: transparent; transition: opacity .9s ease; }
.hero-wordmark.is-webgl .hero-word { opacity: 0; }
.hero-bottom { display: flex; align-items: flex-end; justify-content: space-between; gap: 40px; }
.hero-copy { position: relative; max-width: 620px; }
.hero-copy::before { content: ''; position: absolute; top: -90px; right: -120px; bottom: -70px; left: -90px; z-index: -1; background: var(--home-halo); pointer-events: none; }
.hero-headline { font-size: clamp(30px, 3.4vw, 52px); font-weight: 700; line-height: 1.18; letter-spacing: -.03em; }
.hero-subtitle { margin-top: 14px; font-size: 15px; font-weight: 700; color: var(--home-accent); white-space: pre-wrap; overflow-wrap: anywhere; }
.hero-lead { margin-top: 16px; font-size: 15.5px; line-height: 1.78; color: var(--home-ink-2); max-width: 32em; }
.hero-links { margin-top: 28px; }
.hero-scroll { position: absolute; left: 50%; bottom: 22px; transform: translateX(-50%); display: inline-flex; align-items: center; gap: 12px; font-size: 11px; font-weight: 700; letter-spacing: .24em; text-transform: uppercase; color: var(--home-ink-3); }
.scroll-pill { position: relative; width: 22px; height: 34px; border-radius: 12px; border: 1px solid var(--home-line-2); }
.scroll-pill i { position: absolute; left: 50%; top: 7px; width: 4px; height: 7px; margin-left: -2px; border-radius: 2px; background: var(--home-accent); animation: scroll-hint 2.2s var(--home-ease) infinite; }
@keyframes scroll-hint { 0% { opacity: 0; transform: translateY(-4px); } 30% { opacity: 1; } 70% { opacity: 1; transform: translateY(10px); } 100% { opacity: 0; transform: translateY(12px); } }
.home-hud { position: absolute; inset: 96px clamp(16px, 2.6vw, 40px) 26px; pointer-events: none; }
.home-hud i { position: absolute; width: 22px; height: 22px; border: 0 solid color-mix(in srgb, var(--home-accent) 40%, transparent); }
.home-hud .is-tl { left: 0; top: 0; border-left-width: 1px; border-top-width: 1px; }
.home-hud .is-tr { right: 0; top: 0; border-right-width: 1px; border-top-width: 1px; }
.home-hud .is-bl { left: 0; bottom: 0; border-left-width: 1px; border-bottom-width: 1px; }
.home-hud .is-br { right: 0; bottom: 0; border-right-width: 1px; border-bottom-width: 1px; }
.home-hud span { position: absolute; font: 700 10px var(--home-mono); letter-spacing: .22em; text-transform: uppercase; color: var(--home-ink-3); }
.hud-a { left: 34px; top: 4px; }
.hud-b { right: 34px; top: 4px; }

/* Manifesto */
.manifesto { height: 250vh; padding: 0; }
.manifesto-inner { display: flex; flex-direction: column; gap: 36px; }
.manifesto-text { font-size: clamp(28px, 4.3vw, 64px); font-weight: 700; line-height: 1.34; letter-spacing: -.03em; max-width: 19em; }
:lang(zh) .manifesto-text { letter-spacing: .005em; line-height: 1.5; max-width: 16em; }
.mu { opacity: calc(.13 + .87 * clamp(0, (var(--pp, 0) * 1.3 * (var(--n) + 14) - var(--i)) / 14, 1)); transition: opacity .12s linear; }
.mu.is-hl { color: var(--home-accent); }
.manifesto-sign { display: inline-flex; align-items: center; gap: 14px; font-size: 12px; font-weight: 750; letter-spacing: .3em; text-transform: uppercase; color: var(--home-ink-3); }
.manifesto-sign i { width: 40px; height: 1px; background: var(--home-accent); }

/* Connect */
.split { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: clamp(32px, 5vw, 80px); align-items: center; }
.connect-copy { display: flex; flex-direction: column; gap: 24px; }
.outline-word { position: absolute; left: 0; right: 0; pointer-events: none; user-select: none; font-size: clamp(110px, 21vw, 340px); font-weight: 800; letter-spacing: -.06em; line-height: .8; white-space: nowrap; color: transparent; -webkit-text-stroke: 1px color-mix(in srgb, var(--home-accent) 22%, transparent); transform: translate3d(calc((.5 - var(--sp, .5)) * 34vw), 0, 0); }
.connect-bgword { top: 50%; margin-top: -.4em; opacity: .9; }
.endpoint { padding: 20px 22px; margin-top: 6px; }
.endpoint-row { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; padding-bottom: 16px; border-bottom: 1px solid var(--home-line); }
.endpoint-row code { flex: 1; min-width: 0; overflow-wrap: anywhere; font-size: 14px; color: var(--home-ink); }
.endpoint-copy { height: 32px; padding: 0 14px; border-radius: 10px; border: 1px solid var(--home-line-2); background: transparent; font: 650 12px var(--home-font); color: var(--home-ink-2); cursor: pointer; }
.endpoint-copy:hover { color: var(--home-ink); border-color: var(--home-accent); }
.endpoint-feedback { margin-top: 10px; font-size: 11.5px; color: var(--home-ink-3); }
.stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; padding-top: 16px; }
.stat strong { display: block; font-size: clamp(28px, 3vw, 40px); font-weight: 750; letter-spacing: -.04em; line-height: 1; }
.stat span { display: block; margin-top: 8px; font-size: 12.5px; color: var(--home-ink-3); }
.constellation { position: relative; height: min(520px, 58vh); }
.planet { position: absolute; will-change: transform; }
.planet > span { display: inline-flex; align-items: center; gap: 9px; height: 38px; padding: 0 16px 0 12px; border-radius: 999px; border: 1px solid var(--home-line); background: color-mix(in srgb, var(--home-surface) 85%, transparent); backdrop-filter: blur(6px); font-size: 13.5px; font-weight: 700; white-space: nowrap; }
.planet i { width: 8px; height: 8px; border-radius: 50%; background: var(--home-accent); box-shadow: 0 0 12px var(--home-accent); }
.planet:nth-child(3n) i { background: var(--home-amber); box-shadow: 0 0 12px var(--home-amber); }
.float { animation: float-y 7s ease-in-out infinite; }
@keyframes float-y { 0%, 100% { transform: translateY(0); } 50% { transform: translate3d(0, -10px, 0); } }
.constellation-note { position: absolute; left: 0; right: 0; bottom: -40px; font-size: 11.5px; color: var(--home-ink-3); text-align: center; }

/* Capabilities */
.caps { height: calc(100vh + var(--shift, 0) * 1px + 20vh); height: calc(100svh + var(--shift, 0) * 1px + 20vh); padding: 0; }
.caps-pin { flex-direction: column; justify-content: center; gap: clamp(28px, 5vh, 56px); }
.caps-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 40px; }
.caps-head > div:first-child { display: flex; flex-direction: column; gap: 18px; }
.caps-meter { display: flex; flex-direction: column; align-items: flex-end; gap: 10px; min-width: 220px; font-size: 13px; font-weight: 700; color: var(--home-ink-2); }
.caps-meter i { position: relative; width: 220px; height: 2px; border-radius: 2px; background: var(--home-line); overflow: hidden; }
.caps-meter b { position: absolute; inset: 0; background: linear-gradient(90deg, var(--home-accent-2), var(--home-accent)); transform-origin: 0 50%; transform: scaleX(var(--pp, 0)); }
.caps-meter small { font-size: 11.5px; font-weight: 600; color: var(--home-ink-3); }
.caps-viewport { width: 100%; overflow: visible; }
.caps-track { display: flex; gap: 22px; width: max-content; padding: 0 max(calc((100vw - 1240px) / 2), clamp(18px, 3.5vw, 60px)); transform: translate3d(calc(var(--pp, 0) * var(--shift, 0) * -1px), 0, 0); will-change: transform; }
.cap { flex: 0 0 auto; width: clamp(300px, 27vw, 390px); padding: 26px 26px 30px; display: flex; flex-direction: column; gap: 14px; transform: perspective(1100px) rotateX(var(--rx, 0deg)) rotateY(var(--ry, 0deg)); transition: transform .7s var(--home-ease), border-color .3s; }
.cap.is-tilting { transition: transform .12s linear, border-color .3s; }
.cap-top { display: flex; align-items: center; justify-content: space-between; }
.cap-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--home-accent); box-shadow: 0 0 14px var(--home-accent); }
.cap:nth-child(2) .cap-dot, .cap:nth-child(5) .cap-dot { background: var(--home-amber); box-shadow: 0 0 14px var(--home-amber); }
.cap-visual { height: 150px; margin: 4px 0 10px; border-radius: 18px; border: 1px solid var(--home-line); background: radial-gradient(120% 120% at 50% 0%, var(--home-accent-soft), transparent 70%); display: grid; place-items: center; overflow: hidden; color: var(--home-accent); }
.cap-visual svg { width: 86%; height: auto; overflow: visible; }
.cap .home-body { font-size: 14.5px; }
.hv-line { fill: none; stroke: var(--home-line-2); stroke-width: 1.5; }
.hv-line.is-dim { stroke-dasharray: 3 5; }
.hv-flow { fill: none; stroke: currentColor; stroke-width: 2; stroke-dasharray: 6 8; animation: dash-flow 1.4s linear infinite; }
@keyframes dash-flow { to { stroke-dashoffset: -14; } }
.hv-node { fill: none; stroke: var(--home-line-2); }
.hv-core { fill: currentColor; }
.hv-src { fill: var(--home-ink-3); }
.hv-ok { fill: var(--home-accent); }
.hv-warn { fill: var(--home-amber); }
.hv-keys { width: 82%; display: grid; gap: 10px; }
.hv-key { display: flex; align-items: center; justify-content: space-between; padding: 9px 12px; border-radius: 10px; border: 1px solid var(--home-line-2); font-size: 12px; }
.hv-key i { width: 34px; height: 5px; border-radius: 3px; background: var(--home-accent); opacity: .6; }
.hv-toggle { display: flex; align-items: center; justify-content: space-between; font-size: 12px; color: var(--home-ink-2); }
.hv-toggle b { position: relative; width: 34px; height: 18px; border-radius: 999px; background: color-mix(in srgb, var(--home-accent) 30%, transparent); }
.hv-toggle b::after { content: ''; position: absolute; right: 2px; top: 2px; width: 14px; height: 14px; border-radius: 50%; background: var(--home-accent); }
.hv-bars { display: flex; align-items: flex-end; gap: 6px; height: 100px; padding: 0 4px; }
.hv-bars i { width: 12px; height: calc(var(--h)); border-radius: 3px 3px 1px 1px; background: linear-gradient(180deg, var(--home-accent), color-mix(in srgb, var(--home-accent) 25%, transparent)); animation: bar-in .8s var(--home-ease) calc(var(--n) * 60ms) both; }
@keyframes bar-in { from { transform: scaleY(.2); opacity: .3; } }
.hv-stream { display: grid; gap: 10px; width: 84%; }
.hv-stream span { display: flex; gap: 6px; }
.hv-stream i { height: 7px; border-radius: 4px; background: color-mix(in srgb, var(--home-accent) 35%, transparent); animation: stream-pulse 1.6s ease-in-out calc(var(--n) * 140ms) infinite; }
.hv-stream i:nth-child(1) { width: 34%; } .hv-stream i:nth-child(2) { width: 24%; animation-delay: calc(var(--n) * 140ms + .2s); } .hv-stream i:nth-child(3) { width: 18%; animation-delay: calc(var(--n) * 140ms + .4s); }
@keyframes stream-pulse { 0%, 100% { opacity: .35; } 50% { opacity: 1; } }
.hv-balance { position: relative; display: grid; place-items: center; }
.hv-balance svg { width: 130px; height: 130px; transform: rotate(-90deg); }
.hv-ring { fill: none; stroke: var(--home-line); stroke-width: 6; }
.hv-ring-fill { fill: none; stroke: currentColor; stroke-width: 6; stroke-linecap: round; stroke-dasharray: 276; stroke-dashoffset: 105; animation: ring-sweep 1.2s var(--home-ease) both; }
@keyframes ring-sweep { from { stroke-dashoffset: 276; } }
.hv-balance-value { position: absolute; display: grid; justify-items: center; gap: 4px; }
.hv-balance-value strong { font-size: 20px; font-weight: 750; letter-spacing: -.03em; color: var(--home-ink); }
.hv-balance-value span { font-size: 10.5px; color: var(--home-ink-3); }

/* Warp */
.warp { padding: 0; }
.warp-inner { display: flex; flex-direction: column; align-items: center; gap: 26px; text-align: center; }
.warp-title { display: grid; gap: 6px; font-size: clamp(38px, 6.4vw, 104px); font-weight: 730; line-height: 1.05; letter-spacing: -.05em; }
.warp-tags { display: flex; flex-wrap: wrap; justify-content: center; gap: 10px; }

/* Control */
.is-reversed { direction: rtl; }
.is-reversed > * { direction: ltr; }
.stage { position: relative; min-height: min(560px, 66vh); }
.console { position: relative; width: 100%; height: 100%; display: grid; place-items: center; }
.panel { width: min(420px, 100%); padding: 22px; display: grid; gap: 16px; }
.panel-head { display: flex; align-items: center; justify-content: space-between; }
.panel-metric { display: flex; align-items: center; justify-content: space-between; }
.panel-metric strong { font-size: clamp(26px, 2.6vw, 36px); font-weight: 750; letter-spacing: -.04em; }
.pill, .badge { display: inline-flex; align-items: center; gap: 6px; height: 24px; padding: 0 10px; border-radius: 999px; border: 1px solid color-mix(in srgb, var(--home-accent) 40%, transparent); background: var(--home-accent-soft); font-size: 11px; font-weight: 700; color: var(--home-accent); }
.pill::before, .badge::before { content: ''; width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.panel-chart { display: flex; align-items: flex-end; gap: 5px; height: 92px; }
.panel-chart i { flex: 1; height: calc(var(--h)); border-radius: 3px; background: linear-gradient(180deg, var(--home-accent), color-mix(in srgb, var(--home-accent) 20%, transparent)); animation: bar-in 1s var(--home-ease) calc(var(--n) * 50ms) both; }
.panel-quota { display: flex; align-items: center; gap: 12px; font-size: 12px; color: var(--home-ink-3); }
.panel-quota i { flex: 1; height: 4px; border-radius: 3px; background: var(--home-line); overflow: hidden; }
.panel-quota b { display: block; width: 62%; height: 100%; background: linear-gradient(90deg, var(--home-accent-2), var(--home-accent)); }
.float-card { position: absolute; padding: 12px 14px; border-radius: 16px; font-size: 12px; }
.float-key { right: -2%; top: 12%; display: flex; align-items: center; gap: 10px; }
.float-ring { left: 2%; bottom: 6%; width: 118px; height: 118px; display: grid; place-items: center; color: var(--home-accent); }
.float-ring svg { width: 100%; height: 100%; transform: rotate(-90deg); }
.float-log { right: 4%; bottom: 16%; display: grid; gap: 6px; }
.float-log .row { color: var(--home-ink-3); font-size: 11px; }
.float-log .row.is-active { color: var(--home-accent); }
.control-copy { display: flex; flex-direction: column; gap: 20px; }
.bullets { display: grid; gap: 14px; margin-top: 6px; }
.bullets li { display: flex; align-items: baseline; gap: 14px; font-size: 15px; color: var(--home-ink-2); }
.bullets .num { color: var(--home-accent); }

/* Clients */
.clients { padding: clamp(90px, 12vh, 150px) 0; }
.clients-inner { width: 100%; display: flex; flex-direction: column; gap: clamp(32px, 6vh, 60px); }
.clients-head { display: flex; flex-direction: column; gap: 20px; }
.marquees { display: grid; gap: 18px; }
.marquee { overflow: hidden; mask-image: linear-gradient(90deg, transparent, #000 9%, #000 91%, transparent); -webkit-mask-image: linear-gradient(90deg, transparent, #000 9%, #000 91%, transparent); }
.marquee-track { display: flex; width: max-content; animation: marquee var(--dur, 46s) linear infinite; }
.marquee.is-reverse .marquee-track { animation-direction: reverse; }
.marquee:hover .marquee-track { animation-play-state: paused; }
@keyframes marquee { to { transform: translateX(-50%); } }
.marquee-track ul { display: flex; gap: 14px; padding-right: 14px; }
.marquee-item { display: inline-flex; align-items: center; gap: 14px; height: 72px; padding: 0 34px 0 26px; border-radius: 999px; border: 1px solid var(--home-line); background: var(--home-surface); font-size: clamp(17px, 1.7vw, 23px); font-weight: 700; letter-spacing: -.02em; white-space: nowrap; }
.marquee-item i { width: 9px; height: 9px; border-radius: 50%; background: var(--home-accent); box-shadow: 0 0 14px var(--home-accent); }
.marquee-item:nth-child(3n) i { background: var(--home-amber); box-shadow: 0 0 14px var(--home-amber); }
.clients-note { width: min(1240px, calc(100% - clamp(36px, 7vw, 120px))); margin: 0 auto; font-size: 11.5px; color: var(--home-ink-3); }
.integration-paths { width: min(1240px, calc(100% - clamp(36px, 7vw, 120px))); margin: 6px auto 0; border-top: 1px solid var(--home-line); border-bottom: 1px solid var(--home-line); }
.integration-paths-head { display: flex; align-items: baseline; justify-content: space-between; gap: 24px; padding: 18px 0; color: var(--home-ink-3); }
.integration-paths-head p { margin: 0; font-size: 12px; }
.integration-paths ul { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); border-top: 1px solid var(--home-line); }
.integration-paths li { display: flex; gap: 14px; min-width: 0; padding: 20px 18px 22px 0; }
.integration-paths li + li { padding-left: 18px; border-left: 1px solid var(--home-line); }
.integration-path-index { flex: none; color: var(--home-accent); font: 700 11px var(--home-mono); }
.integration-paths li div { display: grid; gap: 7px; min-width: 0; }
.integration-paths strong { font-size: 14px; font-weight: 700; color: var(--home-ink); }
.integration-paths li div span { font-size: 12px; line-height: 1.65; color: var(--home-ink-3); }

/* Steps */
.steps-grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr); gap: clamp(32px, 6vw, 90px); align-items: center; }
.steps-grid > div:first-child { display: flex; flex-direction: column; gap: 24px; align-items: flex-start; }
.steps-list { display: grid; }
.steps-list li { display: flex; gap: 22px; padding: 26px 0; border-bottom: 1px solid var(--home-line); }
.steps-list li:first-child { border-top: 1px solid var(--home-line); }
.steps-list .num { color: var(--home-accent); line-height: 2; }
.steps-list .home-body { margin-top: 10px; }

/* FAQ */
.faq-grid { display: grid; grid-template-columns: minmax(0, .85fr) minmax(0, 1.15fr); gap: clamp(32px, 6vw, 90px); align-items: center; }
.faq-grid > div:first-child { display: flex; flex-direction: column; gap: 20px; }
.faq-list details { border-bottom: 1px solid var(--home-line); }
.faq-list details:first-child { border-top: 1px solid var(--home-line); }
.faq-list summary { display: flex; align-items: center; gap: 16px; padding: 22px 0; font-size: 15.5px; font-weight: 650; cursor: pointer; list-style: none; }
.faq-list summary::-webkit-details-marker { display: none; }
.faq-list summary b { flex: 1; font-weight: 650; }
.faq-plus { position: relative; width: 16px; height: 16px; flex: none; }
.faq-plus::before, .faq-plus::after { content: ''; position: absolute; left: 0; right: 0; top: 50%; height: 1.5px; background: var(--home-accent); transition: transform .35s var(--home-ease); }
.faq-plus::after { transform: rotate(90deg); }
.faq-list details[open] .faq-plus::after { transform: rotate(0deg); }
.faq-list details p { padding: 0 0 22px 42px; }

/* Finale */
.finale { flex-direction: column; justify-content: center; text-align: center; }
.finale .home-container { display: flex; flex-direction: column; align-items: center; gap: 26px; }
.finale .btn-row { justify-content: center; }

/* Footer */
.home-footer { position: relative; z-index: 2; padding: clamp(60px, 9vh, 110px) 0 34px; border-top: 1px solid var(--home-line); background: color-mix(in srgb, var(--home-bg) 92%, transparent); overflow: hidden; }
.home-footer-glow { position: absolute; inset: -60% 0 auto; height: 60%; background: radial-gradient(60% 100% at 50% 100%, color-mix(in srgb, var(--home-accent) 12%, transparent), transparent 70%); pointer-events: none; }
.home-footer-top { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(0, 1.6fr); gap: clamp(30px, 5vw, 80px); }
.footer-brand { display: flex; flex-direction: column; gap: 18px; }
.footer-lead { font-size: 13.5px; line-height: 1.8; color: var(--home-ink-3); max-width: 30em; }
.footer-cols { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 24px; }
.footer-cols h3 { margin-bottom: 14px; font: 700 10.5px var(--home-mono); letter-spacing: .2em; text-transform: uppercase; color: var(--home-ink-3); }
.footer-cols a { display: block; padding: 6px 0; font-size: 13.5px; color: var(--home-ink-2); }
.footer-cols a:hover { color: var(--home-accent); }
.home-footer-word { display: flex; justify-content: center; margin: clamp(30px, 6vh, 70px) 0 10px; pointer-events: none; user-select: none; }
.home-footer-word span { font-size: clamp(60px, 17vw, 260px); font-weight: 800; letter-spacing: -.06em; line-height: .8; color: transparent; -webkit-text-stroke: 1px color-mix(in srgb, var(--home-accent) 24%, transparent); will-change: transform; }
.home-footer-bottom { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding-top: 20px; font-size: 12px; color: var(--home-ink-3); }
.top-btn { display: inline-flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: 999px; border: 1px solid var(--home-line); background: transparent; font: 650 12px var(--home-font); color: var(--home-ink-2); cursor: pointer; }
.top-btn:hover { color: var(--home-ink); border-color: var(--home-accent); }
.top-btn svg { width: 14px; height: 14px; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }

/* Motion toggle */
.orbit-motion { position: fixed; right: 18px; bottom: 18px; z-index: 45; display: inline-flex; align-items: center; gap: 8px; height: 38px; padding: 0 14px 0 12px; border-radius: 999px; border: 1px solid var(--home-line); background: var(--home-surface); color: var(--home-ink-2); font: 600 12px var(--home-font); cursor: pointer; backdrop-filter: blur(8px); }
.orbit-motion:hover { color: var(--home-ink); border-color: var(--home-line-2); }

/* Reduced motion */
.motion-disabled [data-reveal] { opacity: 1 !important; transform: none !important; transition: none !important; }
.motion-disabled :deep(.split-ch) { opacity: 1 !important; transform: none !important; transition: none !important; }
.motion-disabled .float, .motion-disabled .marquee-track, .motion-disabled .scroll-pill i, .motion-disabled .hv-flow, .motion-disabled .hv-bars i, .motion-disabled .hv-ring-fill, .motion-disabled .hv-stream i, .motion-disabled .panel-chart i { animation: none !important; }
.motion-disabled .home-btn, .motion-disabled .home-cta { transform: none !important; }

@media (max-width: 1100px) {
  .home-nav { display: none; }
  .home-actions, .home-nav + .home-actions { margin-left: auto; }
  .home-menu-btn { display: block; flex-shrink: 0; }
}
@media (max-width: 899px) {
  .home-header { padding: 10px 14px 0; }
  .home-header-bar { height: 58px; padding: 0 8px 0 10px; gap: 10px; }
  .home-brand-text small { display: none; }
  .home-rail { display: none; }
  .home-actions .home-login { display: none; }
  .home-menu-btn { display: block; margin-left: auto; }
  .home-chapter { min-height: auto; padding: 84px 0 64px; }
  .hero { min-height: 100svh; display: flex; padding: 0; }
  .hero-inner { min-height: 100svh; padding: 96px 0 80px; }
  .hero-word { font-size: clamp(52px, 19vw, 120px); }
  .hero-wordmark { top: 40svh; }
  .hero-kicker { font-size: 9.5px; letter-spacing: .26em; margin-top: 0; }
  .hero-bottom { flex-direction: column; align-items: stretch; margin-top: max(260px, 36svh); }
  .hero-headline { font-size: clamp(28px, 7.4vw, 40px); }
  .home-hud { inset: 78px 14px 20px; }
  .hud-b { display: none; }
  .manifesto { height: auto; padding: 84px 0; }
  .manifesto-inner { gap: 24px; }
  .split, .steps-grid, .faq-grid, .home-footer-top { grid-template-columns: minmax(0, 1fr); }
  .is-reversed { direction: ltr; }
  .constellation { height: 320px; margin-top: 20px; }
  .integration-paths-head { display: grid; gap: 8px; }
  .integration-paths ul { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .integration-paths li:nth-child(3) { padding-left: 0; border-left: 0; border-top: 1px solid var(--home-line); }
  .integration-paths li:nth-child(4) { border-top: 1px solid var(--home-line); }
  .caps { height: calc(100svh + var(--shift, 0) * 1px + 16vh); }
  .caps-head { flex-direction: column; align-items: flex-start; gap: 18px; }
  .caps-meter { align-items: flex-start; min-width: 0; }
  .caps-meter i { width: 160px; }
  .cap { width: min(76vw, 320px); }
  .stage { min-height: 420px; }
  .float-card { transform: scale(.88); }
  .warp-title { font-size: clamp(32px, 9vw, 54px); }
  .home-display { font-size: clamp(38px, 11vw, 62px); }
  .home-h2 { font-size: clamp(30px, 8.4vw, 46px); }
  .footer-cols { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .home-footer-bottom { flex-direction: column; align-items: flex-start; }
  .orbit-motion { right: 12px; bottom: 12px; }
}
@media (max-width: 600px) {
  .home-actions .home-cta { display: none; }
  .home-brand { min-width: 0; flex-shrink: 1; }
  .home-brand-mark, .home-brand-img { flex-shrink: 0; }
  .home-header-bar { gap: 6px; }
  .home-menu-btn { margin-left: 0; }
}
@media (prefers-reduced-motion: reduce) {
  .float, .marquee-track, .scroll-pill i, .hv-flow, .hv-bars i, .hv-ring-fill, .hv-stream i, .panel-chart i { animation: none !important; }
}
</style>
