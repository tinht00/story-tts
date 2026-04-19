import { createRequire } from 'node:module'
import fs from 'node:fs'
import path from 'node:path'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

function serializeDefineForClient(defines: Record<string, unknown>) {
  const entries = Object.entries(defines)
  if (entries.length === 0) {
    return '{}'
  }

  return `{${entries
    .map(([key, value]) => `${JSON.stringify(key)}: ${typeof value === 'string' ? value : JSON.stringify(value)}`)
    .join(',')}}`
}

function fixViteInternalClientRoutes() {
  const require = createRequire(import.meta.url)
  const vitePackageDir = path.dirname(require.resolve('vite/package.json'))
  const viteClientDir = path.join(vitePackageDir, 'dist', 'client')
  const viteClientSource = fs.readFileSync(path.join(viteClientDir, 'client.mjs'), 'utf-8')
  const viteEnvSource = fs.readFileSync(path.join(viteClientDir, 'env.mjs'), 'utf-8')

  return {
    name: 'story-tts:fix-vite-internal-client-routes',
    configureServer(server: any) {
      server.middlewares.use((req: any, res: any, next: any) => {
        const requestUrl = req?.url ?? ''
        const url = requestUrl.split('?')[0]

        if (url !== '/@vite/client' && url !== '/@vite/env') {
          next()
          return
        }

        const hostHeader = String(req?.headers?.host ?? '')
        const [requestHostname, requestPort] = hostHeader.split(':')
        const resolvedHostname = requestHostname || 'localhost'
        const resolvedPort = Number(requestPort) || server.config.server.port || 5173
        const devBase = server.config.base
        const hmrConfig =
          server.config.server.hmr && typeof server.config.server.hmr === 'object'
            ? server.config.server.hmr
            : undefined

        let clientCode = viteClientSource.replace('import "@vite/env";', 'import "/@vite/env";')
        let envCode = viteEnvSource

        const userDefine = Object.fromEntries(
          Object.entries(server.config.define ?? {}).filter(([key]) => !key.startsWith('import.meta.env.'))
        )

        const hmrConfigName = path.basename(server.config.configFile || 'vite.config.ts')
        let hmrPort = hmrConfig?.clientPort ?? hmrConfig?.port ?? null
        if (server.config.server.middlewareMode && !hmrConfig?.server) {
          hmrPort ??= 24678
        }

        let hmrBase = devBase
        if (hmrConfig?.path) {
          hmrBase = path.posix.join(hmrBase, hmrConfig.path)
        }

        const replacements = new Map<string, unknown>([
          ['__BASE__', devBase],
          ['__SERVER_HOST__', `${resolvedHostname}:${resolvedPort}${devBase}`],
          ['__HMR_PROTOCOL__', hmrConfig?.protocol ?? null],
          ['__HMR_HOSTNAME__', hmrConfig?.host ?? null],
          ['__HMR_PORT__', hmrPort],
          ['__HMR_DIRECT_TARGET__', `${hmrConfig?.host ?? resolvedHostname}:${hmrConfig?.port ?? resolvedPort}${devBase}`],
          ['__HMR_BASE__', hmrBase],
          ['__HMR_TIMEOUT__', hmrConfig?.timeout ?? 30000],
          ['__HMR_ENABLE_OVERLAY__', hmrConfig?.overlay !== false],
          ['__HMR_CONFIG_NAME__', hmrConfigName],
          ['__WS_TOKEN__', server.config.webSocketToken]
        ])

        for (const [placeholder, value] of replacements) {
          clientCode = clientCode.replaceAll(placeholder, JSON.stringify(value))
        }

        envCode = envCode.replace('__DEFINES__', serializeDefineForClient(userDefine))

        res.statusCode = 200
        res.setHeader('Content-Type', 'text/javascript')
        res.end(url === '/@vite/client' ? clientCode : envCode)
      })
    }
  }
}

export default defineConfig({
  plugins: [fixViteInternalClientRoutes(), vue()],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:18080',
      '/health': 'http://127.0.0.1:18080'
    }
  }
})
