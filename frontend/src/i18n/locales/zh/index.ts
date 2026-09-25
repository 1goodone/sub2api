import qualityOps from './qualityOps'
import accountOps from './accountOps'
import requestTiming from './requestTiming'
import tokenGuard from './tokenGuard'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import channelMonitorV3 from './channelMonitorV3'
import batchImage from './batchImage'
import keyPackages from './keyPackages'
import admin from './admin'
import misc from './misc'

export default {
  qualityOps,
  accountOps,
  tokenGuard,
  requestTiming,
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...channelMonitorV3,
  ...batchImage,
  ...keyPackages,
  admin,
  ...misc,
}
