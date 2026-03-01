import { Icon } from "@meowhomo/ui"
import "./App.css"

function App() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-slate-900 text-slate-100 p-8 space-y-8">
      <h1 className="text-4xl font-bold flex items-center gap-3">
        <Icon name="logo" className="w-12 h-12 text-cyan-400 animate-pulse" />
        MeowHomo
      </h1>

      <div className="flex gap-4 p-6 bg-slate-800 rounded-xl border border-slate-700 shadow-xl">
        <div className="flex flex-col items-center gap-2 p-4 hover:bg-slate-700 rounded-lg transition-colors cursor-pointer group">
          <Icon name="nav-dashboard" className="w-8 h-8 text-slate-300 group-hover:text-cyan-400 group-hover:rotate-12 transition-all" />
          <span className="text-sm font-medium">仪表板</span>
        </div>
      </div>

      <p className="text-slate-400 text-sm max-w-md text-center">
        将 SVG 放入 <code className="text-cyan-300">src/assets/svg/</code> 目录，
        即可使用 <code className="text-cyan-300">&lt;Icon name=&quot;文件名&quot; /&gt;</code> 调用，支持 className 传色和 hover 动效！
      </p>
    </div>
  )
}

export default App
