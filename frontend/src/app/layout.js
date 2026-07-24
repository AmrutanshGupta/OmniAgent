import "./globals.css";

export const metadata = {
  title: "OmniAgent — Infrastructure-Aware LLM Orchestrator",
  description: "Coordinator-Worker LLM orchestration with ToT planning and FrugalGPT cascades",
};

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
