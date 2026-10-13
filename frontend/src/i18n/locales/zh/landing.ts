export default {
  batchImageGuide: {
    title: '图片批量生成',
    description: '一次提交多条提示词，任务完成后可统一下载图片结果'
  },
  // Home Page
  home: {
    orbit: {
      skip: '跳到主要内容',
      navigation: '首页导航',
      chapters: '页面章节',
      platform: '平台能力',
      ecosystem: '开发者生态',
      start: '开始使用',
      faq: '常见问题',
      kicker: 'Intelligence · In your orbit',
      hudA: '轨道 01',
      hudB: '∠ 35° · ORBIT',
      headline: '让智能，[[进入你的轨道]]。',
      heroLead: '让每一个想法，',
      heroAccent: '连接无限可能。',
      lead: '一个入口，连接你的 AI 宇宙。统一模型接入、密钥与用量管理，让每一次调用都稳稳在轨。',
      explore: '探索平台',
      pause: '暂停动效',
      resume: '播放动效',
      scroll: '向下滚动 · 进入轨道',
      manifestoLabel: '宣言 · Manifesto',
      manifesto: '模型在变，协议在变，世界每天都在加速。我们把纷繁的智能收束成[[一条清晰的轨道]]，你只需要专注于[[创造本身]]。',
      connectLabel: '统一接入',
      connectTitle: '万千模型，\n[[一个入口]]。',
      connectDescription: '兼容 OpenAI、Anthropic 与 Gemini 等主流接口格式。更换模型不必重写代码，现有 SDK 与客户端改一个地址即可接入。',
      chipOpenai: 'OpenAI 兼容',
      chipStream: '流式 SSE',
      endpoint: 'Base URL',
      endpointHint: '复制地址，配置到支持自定义接口的客户端。',
      copyFailed: '复制未成功，请手动选择并复制地址。',
      statEndpoint: '统一入口',
      statFormats: '主流接口格式',
      statCombos: '组合可能',
      modelsNote: '模型名称仅示意兼容范围，实际可用模型以控制台分组为准。',
      capsLabel: '能力矩阵',
      capsTitle: '为每一次调用，\n[[校准轨道]]。',
      capsCount: '6 项核心能力',
      capsHint: '继续滚动，横向浏览',
      capAccess: '统一接入',
      capAccessDesc: '一个 Base URL 汇聚多家模型。标准接口格式，主流 SDK 与客户端开箱即用。',
      capRouting: '智能调度',
      capRoutingDesc: '请求在可用通道间自动调度，遇到异常即时切换，单点波动不打断你的工作流。',
      capKeys: '密钥管控',
      capKeysDesc: '按项目创建密钥，精细设置额度、有效期、IP 白名单与速率限制。',
      capUsage: '用量透明',
      capUsageDesc: '每一次请求的模型、Token 与费用都有据可查，账单清清楚楚。',
      capStream: '流式直达',
      capStreamDesc: '完整支持流式输出，长回答也能逐字实时抵达。',
      capBalance: '按量计费',
      capBalanceDesc: '余额与订阅灵活组合，用多少付多少，额度上限随时可调。',
      keyQuota: '额度上限',
      keyExpiry: '有效期',
      keyAllowlist: 'IP 白名单',
      warpLabel: '实时',
      warpTitleA: '让每一个 Token，',
      warpTitleB: '实时抵达。',
      warpLead: '从第一个字符到最后一行代码，流式响应全链路直达。',
      controlLabel: '工作台',
      controlTitle: '复杂，\n[[归于秩序]]。',
      controlDescription: '在同一个工作台里管理密钥、分组、用量与订阅。把繁琐交给平台，把注意力留给创造。',
      bullet1: '项目独立密钥，按需启停',
      bullet2: '额度与速率限制，用量可控',
      bullet3: '请求级用量记录，随时追溯',
      bullet4: '余额与订阅，灵活管理',
      panelTitle: '用量概览',
      panelToday: '今日请求',
      panelProject: '项目 Alpha',
      panelActive: '启用中',
      panelQuota: '额度使用',
      panelLogs: '请求记录',
      clientsLabel: '生态',
      clientsTitle: '你喜欢的工具，\n[[已在轨道上]]。',
      clientsDescription: '凡是支持自定义接口地址的客户端与框架，都能直接接入。',
      clientsNote: '名称仅用于说明兼容性，相关商标归各自所有者所有。',
      integrationLabel: '\u8FDE\u63A5\u65B9\u5F0F',
      integrationDescription: '\u4ECE\u73B0\u6709\u4EE3\u7801\u5230\u81EA\u52A8\u5316\u5DE5\u4F5C\u6D41\uFF0C\u9009\u62E9\u6700\u987A\u624B\u7684\u5165\u53E3\u3002',
      integrationSdk: 'SDK \u63A5\u5165',
      integrationSdkDesc: '\u66FF\u6362 Base URL\uFF0C\u4FDD\u7559\u73B0\u6709\u8C03\u7528\u65B9\u5F0F\u3002',
      integrationCli: '\u547D\u4EE4\u884C\u5DE5\u5177',
      integrationCliDesc: '\u4E3A\u811A\u672C\u548C\u5F00\u53D1\u73AF\u5883\u63D0\u4F9B\u7EDF\u4E00\u51FA\u53E3\u3002',
      integrationWorkflow: '\u5DE5\u4F5C\u6D41\u81EA\u52A8\u5316',
      integrationWorkflowDesc: '\u63A5\u5165 Dify\u3001n8n \u7B49\u53EF\u89C6\u5316\u6D41\u7A0B\u3002',
      integrationCustom: '\u81EA\u5B9A\u4E49\u5E94\u7528',
      integrationCustomDesc: '\u6309\u9879\u76EE\u521B\u5EFA\u5BC6\u94A5\uFF0C\u63A7\u5236\u989D\u5EA6\u4E0E\u7528\u91CF\u3002',
      startLabel: '启程',
      startTitle: '三步，[[进入轨道]]。',
      step1Title: '进入你的工作台',
      step1Description: '登录账号；开放注册时，也可以先创建账号。',
      step2Title: '创建项目密钥',
      step2Description: '选择可用分组，按项目设置密钥与用量额度。',
      step3Title: '连接你的工具',
      step3Description: '填入接口地址与密钥，按照所选模型的文档完成配置。',
      faqLabel: '答疑',
      faqTitle: '出发之前。',
      faqDescription: '让每一次连接，都有清晰的起点。',
      question1: '如何开始使用？',
      answer1: '创建账号并登录控制台，然后按可用分组和账户额度配置密钥。具体注册要求以注册页面为准。',
      registrationClosed: '本站目前未开放注册。已有账号可直接登录；新账号请联系站点管理员。',
      question2: '可以使用哪些模型？',
      answer2: '可用模型取决于账号可访问的分组和站点配置，请以控制台中的实际信息为准。',
      question3: '如何查看用量与费用？',
      answer3: '登录后可以在用量页面查看请求记录、Token 和费用。余额及可用订阅以你的账户信息为准。',
      question4: '现有代码需要改动吗？',
      answer4: '通常只需要把 SDK 或客户端的 Base URL 换成本站地址，并填入在控制台创建的密钥。',
      finaleTitle: '下一次创造，\n[[由你定义]]。',
      finaleLead: '宇宙很大。你的下一步，就从这里开始。',
      backToTop: '回到顶部',
      chapterHero: '起点',
      chapterManifesto: '宣言',
      chapterConnect: '统一接入',
      chapterCaps: '能力矩阵',
      chapterWarp: '实时',
      chapterControl: '工作台',
      chapterClients: '生态',
      chapterStart: '启程',
      chapterFaq: '答疑',
      chapterFinale: '出发',
    },
    viewOnGithub: '在 GitHub 上查看',
    viewDocs: '查看文档',
    docs: '文档',
    switchToLight: '切换到浅色模式',
    switchToDark: '切换到深色模式',
    dashboard: '控制台',
    login: '登录',
    desktopDownload: '下载桌面端',
    getStarted: '立即开始',
    goToDashboard: '进入控制台',
    // 新增：面向用户的价值主张
    heroSubtitle: '一个密钥，畅用多个 AI 模型',
    heroDescription: '无需管理多个订阅账号，一站式接入 Claude、GPT、Gemini 等主流 AI 服务',
    tags: {
      subscriptionToApi: '订阅转 API',
      stickySession: '会话保持',
      realtimeBilling: '按量计费'
    },
    // 用户痛点区块
    painPoints: {
      title: '你是否也遇到这些问题？',
      items: {
        expensive: {
          title: '订阅费用高',
          desc: '每个 AI 服务都要单独订阅，每月支出越来越多'
        },
        complex: {
          title: '多账号难管理',
          desc: '不同平台的账号、密钥分散各处，管理起来很麻烦'
        },
        unstable: {
          title: '服务不稳定',
          desc: '单一账号容易触发限制，影响正常使用'
        },
        noControl: {
          title: '用量无法控制',
          desc: '不知道钱花在哪了，也无法限制团队成员的使用'
        }
      }
    },
    // 解决方案区块
    solutions: {
      title: '我们帮你解决',
      subtitle: '简单三步，开始省心使用 AI'
    },
    features: {
      unifiedGateway: '一键接入',
      unifiedGatewayDesc: '获取一个 API 密钥，即可调用所有已接入的 AI 模型，无需分别申请。',
      multiAccount: '稳定可靠',
      multiAccountDesc: '智能调度多个上游账号，自动切换和负载均衡，告别频繁报错。',
      balanceQuota: '用多少付多少',
      balanceQuotaDesc: '按实际使用量计费，支持设置配额上限，团队用量一目了然。'
    },
    // 优势对比
    comparison: {
      title: '为什么选择我们？',
      headers: {
        feature: '对比项',
        official: '官方订阅',
        us: '本平台'
      },
      items: {
        pricing: {
          feature: '付费方式',
          official: '固定月费，用不完也付',
          us: '按量付费，用多少付多少'
        },
        models: {
          feature: '模型选择',
          official: '单一服务商',
          us: '多模型随意切换'
        },
        management: {
          feature: '账号管理',
          official: '每个服务单独管理',
          us: '统一密钥，一站管理'
        },
        stability: {
          feature: '服务稳定性',
          official: '单账号易触发限制',
          us: '多账号池，自动切换'
        },
        control: {
          feature: '用量控制',
          official: '无法限制',
          us: '可设配额、查明细'
        }
      }
    },
    providers: {
      title: '已支持的 AI 模型',
      description: '一个 API，多种选择',
      supported: '已支持',
      soon: '即将推出',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: '更多'
    },
    // CTA 区块
    cta: {
      title: '准备好开始了吗？',
      description: '注册即可获得免费试用额度，体验一站式 AI 服务',
      button: '免费注册'
    },
    footer: {
      allRightsReserved: '保留所有权利。'
    }
  },

  // Desktop companion download page
  desktop: {
    title: '小羊咩咩桌面端',
    subtitle: '把余额提醒和站点配置带到桌面上。支持 Windows 与 macOS。',
    description: '小羊咩咩会常驻桌面，在余额低于阈值时提醒你，并可协助完成站点连接、分组选择和应用配置。',
    latestRelease: '下载最新版本',
    releaseNote: '选择你的系统后会直接下载安装包；浏览器会要求你确认并手动启动安装。',
    windows: 'Windows',
    windowsDetail: 'Windows 10/11 · x64',
    windowsArch: 'x64',
    macArm: 'macOS Apple Silicon',
    macArmDetail: 'M 系列芯片',
    macArmArch: 'arm64',
    macIntel: 'macOS Intel',
    macIntelDetail: 'Intel 芯片',
    macIntelArch: 'x64',
    features: {
      balance: '余额提醒',
      balanceDetail: '默认低于 ¥5 时提醒，可自定义阈值。',
      setup: '自动配置',
      setupDetail: '连接站点、选择分组并验证模型可用性。',
      secure: '本机安全存储',
      secureDetail: 'API Key 写入系统凭据库，不保存账号密码。',
      update: '自动更新',
      updateDetail: '后台检查签名版本，确认后安装并重启。'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key 用量查询',
    subtitle: '输入您的 API Key 以查看实时消费金额与使用状态',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: '查询',
    querying: '查询中...',
    privacyNote: '您的 Key 仅在浏览器本地处理，不会被存储',
    dateRange: '统计范围:',
    dateRangeToday: '今日',
    dateRange7d: '7 天',
    dateRange30d: '30 天',
    dateRange90d: '90 天',
    dateRangeCustom: '自定义',
    apply: '应用',
    used: '已使用',
    detailInfo: '详细信息',
    tokenStats: 'Token 统计',
    dailyDetail: '按日明细',
    modelStats: '模型用量统计',
    // Table headers
    date: '日期',
    model: '模型',
    requests: '请求数',
    inputTokens: '输入 Tokens',
    outputTokens: '输出 Tokens',
    cacheCreationTokens: '缓存创建',
    cacheReadTokens: '缓存读取',
    cacheWriteTokens: '缓存写入',
    totalTokens: '总 Tokens',
    cost: '费用',
    // Status
    quotaMode: 'Key 限额模式',
    walletBalance: '钱包余额',
    // Ring card titles
    totalQuota: '总额度',
    limit5h: '5 小时限额',
    limitDaily: '日限额',
    limit7d: '7 天限额',
    limitWeekly: '周限额',
    limitMonthly: '月限额',
    // Detail rows
    remainingQuota: '剩余额度',
    expiresAt: '过期时间',
    todayExpires: '(今日到期)',
    daysLeft: '({days} 天)',
    usedQuota: '已用额度',
    resetNow: '即将重置',
    subscriptionType: '订阅类型',
    billingType: '计费方式',
    subscriptionExpires: '订阅到期',
    // Usage stat cells
    todayRequests: '今日请求',
    todayInputTokens: '今日输入',
    todayOutputTokens: '今日输出',
    todayTokens: '今日 Tokens',
    todayCacheCreation: '今日缓存创建',
    todayCacheRead: '今日缓存读取',
    todayCost: '今日费用',
    rpmTpm: 'RPM / TPM',
    totalRequests: '累计请求',
    totalInputTokens: '累计输入',
    totalOutputTokens: '累计输出',
    totalTokensLabel: '累计 Tokens',
    totalCacheCreation: '累计缓存创建',
    totalCacheRead: '累计缓存读取',
    totalCost: '累计费用',
    avgDuration: '平均耗时',
    // Messages
    enterApiKey: '请输入 API Key',
    querySuccess: '查询成功',
    queryFailed: '查询失败',
    queryFailedRetry: '查询失败，请稍后重试',
    noDailyUsage: '暂无按日用量数据',
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API 安装向导',
    description: '配置您的 Sub2API 实例',
    database: {
      title: '数据库配置',
      description: '连接到您的 PostgreSQL 数据库',
      host: '主机',
      port: '端口',
      username: '用户名',
      password: '密码',
      databaseName: '数据库名称',
      sslMode: 'SSL 模式',
      passwordPlaceholder: '密码',
      ssl: {
        disable: '禁用',
        require: '要求',
        verifyCa: '验证 CA',
        verifyFull: '完全验证'
      }
    },
    redis: {
      title: 'Redis 配置',
      description: '连接到您的 Redis 服务器',
      host: '主机',
      port: '端口',
      username: '用户名（可选）',
      password: '密码（可选）',
      database: '数据库',
      usernamePlaceholder: '默认用户留空',
      passwordPlaceholder: '密码',
      enableTls: '启用 TLS',
      enableTlsHint: '连接 Redis 时使用 TLS（公共 CA 证书）'
    },
    admin: {
      title: '管理员账户',
      description: '创建您的管理员账户',
      email: '邮箱',
      password: '密码',
      confirmPassword: '确认密码',
      passwordPlaceholder: '至少 8 个字符',
      confirmPasswordPlaceholder: '确认密码',
      passwordMismatch: '密码不匹配'
    },
    ready: {
      title: '准备安装',
      description: '检查您的配置并完成安装',
      database: '数据库',
      redis: 'Redis',
      adminEmail: '管理员邮箱'
    },
    status: {
      testing: '测试中...',
      success: '连接成功',
      testConnection: '测试连接',
      installing: '安装中...',
      completeInstallation: '完成安装',
      completed: '安装完成！',
      redirecting: '正在跳转到登录页面...',
      restarting: '服务正在重启，请稍候...',
      timeout: '服务重启时间超出预期，请手动刷新页面。'
    }
  },

  // Common
}
