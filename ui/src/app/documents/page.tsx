"use client";

import { useState } from "react";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const mockDocs = [
  { id: "d1", name: "Insurance Policy v4.2", type: "pdf", size: 2400000, category: "policy", chunks: 48, status: "indexed" },
  { id: "d2", name: "Billing FAQ 2026", type: "md", size: 85000, category: "faq", chunks: 12, status: "indexed" },
  { id: "d3", name: "Product Catalog", type: "pdf", size: 5600000, category: "product", chunks: 94, status: "indexed" },
  { id: "d4", name: "Compliance Guidelines", type: "pdf", size: 1200000, category: "procedure", chunks: 28, status: "indexed" },
  { id: "d5", name: "Q2 Promotions", type: "txt", size: 34000, category: "product", chunks: 0, status: "processing" },
];

const mockSearchResults = [
  { chunk: "Section 4.2.1: Water damage from burst pipes is covered under the standard homeowner policy. The policyholder must file a claim within 30 days of the incident.", score: 0.94, doc: "Insurance Policy v4.2" },
  { chunk: "Deductible for water damage claims under Tier 2 coverage is $500. Tier 3 policyholders have a $250 deductible with additional flood coverage.", score: 0.87, doc: "Insurance Policy v4.2" },
  { chunk: "FAQ: Does my policy cover water damage? — Yes, accidental water damage from internal plumbing failures is covered. Flood damage requires a separate rider.", score: 0.81, doc: "Billing FAQ 2026" },
];

const statusStyle: Record<string, string> = {
  indexed: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  processing: "bg-amber-500/20 text-amber-400 border-amber-500/30",
  failed: "bg-rose-500/20 text-rose-400 border-rose-500/30",
};

const categoryStyle: Record<string, string> = {
  policy: "bg-blue-500/15 text-blue-400 border-blue-500/25",
  faq: "bg-cyan-500/15 text-cyan-400 border-cyan-500/25",
  product: "bg-violet-500/15 text-violet-400 border-violet-500/25",
  procedure: "bg-amber-500/15 text-amber-400 border-amber-500/25",
};

function formatSize(bytes: number) {
  if (bytes > 1000000) return `${(bytes / 1000000).toFixed(1)} MB`;
  return `${(bytes / 1000).toFixed(0)} KB`;
}

export default function DocumentsPage() {
  const [query, setQuery] = useState("");
  const [showResults, setShowResults] = useState(false);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-100">Knowledge Base</h1>
        <p className="text-sm text-slate-500 font-mono mt-1">RAG-powered document retrieval for agent assist</p>
      </div>

      <Tabs defaultValue="documents" className="space-y-4">
        <TabsList className="bg-[#0f1629] border border-cyan-500/10">
          <TabsTrigger value="documents" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            DOCUMENTS
          </TabsTrigger>
          <TabsTrigger value="search" className="font-mono text-xs data-[state=active]:bg-cyan-500/15 data-[state=active]:text-cyan-400">
            RAG SEARCH
          </TabsTrigger>
        </TabsList>

        <TabsContent value="documents" className="space-y-4">
          {/* Upload zone */}
          <Card className="bg-[#0f1629] border-cyan-500/10 border-dashed glow-border p-8">
            <div className="text-center">
              <div className="w-12 h-12 mx-auto mb-3 rounded-lg bg-cyan-500/10 flex items-center justify-center">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#06b6d4" strokeWidth="1.5">
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                  <polyline points="17,8 12,3 7,8" />
                  <line x1="12" y1="3" x2="12" y2="15" />
                </svg>
              </div>
              <p className="text-sm text-slate-400 mb-1">Drop PDF, TXT, or MD files here</p>
              <p className="text-xs font-mono text-slate-600">Documents are chunked and indexed in ChromaDB for real-time retrieval</p>
              <Button className="mt-4 bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider">
                BROWSE FILES
              </Button>
            </div>
          </Card>

          {/* Document list */}
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-[11px] font-mono text-slate-500 uppercase tracking-wider border-b border-white/5">
                  <th className="text-left px-5 py-3 font-medium">Document</th>
                  <th className="text-left px-5 py-3 font-medium">Category</th>
                  <th className="text-left px-5 py-3 font-medium">Size</th>
                  <th className="text-left px-5 py-3 font-medium">Chunks</th>
                  <th className="text-left px-5 py-3 font-medium">Status</th>
                </tr>
              </thead>
              <tbody>
                {mockDocs.map((d) => (
                  <tr key={d.id} className="border-b border-white/[0.03] hover:bg-cyan-500/[0.03] transition-colors">
                    <td className="px-5 py-3.5">
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded bg-slate-800 flex items-center justify-center">
                          <span className="text-[10px] font-mono text-slate-400 uppercase">{d.type}</span>
                        </div>
                        <span className="text-slate-200 font-medium">{d.name}</span>
                      </div>
                    </td>
                    <td className="px-5 py-3.5">
                      <Badge variant="outline" className={`text-[10px] font-mono ${categoryStyle[d.category]}`}>
                        {d.category}
                      </Badge>
                    </td>
                    <td className="px-5 py-3.5 font-mono text-xs text-slate-500">{formatSize(d.size)}</td>
                    <td className="px-5 py-3.5 font-mono text-sm text-cyan-400">{d.chunks || "..."}</td>
                    <td className="px-5 py-3.5">
                      <Badge variant="outline" className={`text-[10px] font-mono uppercase ${statusStyle[d.status]}`}>
                        {d.status}
                      </Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>
        </TabsContent>

        <TabsContent value="search" className="space-y-4">
          <Card className="bg-[#0f1629] border-cyan-500/10 glow-border p-5">
            <label className="text-[10px] font-mono text-slate-500 tracking-wider block mb-2">RAG QUERY TEST</label>
            <div className="flex gap-3">
              <Input
                placeholder="Does my insurance cover water damage from a burst pipe?"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                className="bg-[#070b14] border-cyan-500/15 text-slate-200 font-mono text-sm placeholder:text-slate-600"
              />
              <Button
                onClick={() => setShowResults(true)}
                className="bg-cyan-600 hover:bg-cyan-500 text-white font-mono text-xs tracking-wider px-6"
              >
                SEARCH
              </Button>
            </div>
          </Card>

          {showResults && (
            <div className="space-y-3">
              <p className="text-xs font-mono text-slate-500">{mockSearchResults.length} chunks retrieved</p>
              {mockSearchResults.map((r, i) => (
                <Card key={i} className="bg-[#0f1629] border-cyan-500/10 p-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs font-mono text-slate-500">{r.doc}</span>
                    <Badge variant="outline" className="bg-cyan-500/15 text-cyan-400 border-cyan-500/25 font-mono text-[10px]">
                      {Math.round(r.score * 100)}% match
                    </Badge>
                  </div>
                  <p className="text-sm text-slate-300 leading-relaxed">{r.chunk}</p>
                </Card>
              ))}
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
