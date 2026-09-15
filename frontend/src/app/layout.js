// frontend/src/app/layout.js
import "./globals.css";
import Providers from "./Providers";

export const metadata = {
  title: "OmniAgent Orchestrator",
  description: "Advanced Multi-Agent Network and Processing Engine",
};

export default function RootLayout({ children }) {
  return (
    <html lang="en" className="antialiased">
      <body className="font-sans bg-[#F8FAFC] text-slate-900 selection:bg-blue-500/20">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}