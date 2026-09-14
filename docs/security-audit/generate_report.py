#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
HydraStream Security Audit & Remediation PDF Report Generator
Generates a comprehensive executive & technical security audit and remediation report in pt-BR.
"""

import os
import sys
from datetime import datetime
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

from reportlab.lib.pagesizes import A4
from reportlab.lib import colors
from reportlab.lib.units import mm
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle, Image, KeepTogether, HRFlowable
)
from reportlab.pdfgen import canvas

class NumberedCanvas(canvas.Canvas):
    """Two-pass canvas for dynamic total page count in footer."""
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._saved_page_states = []

    def showPage(self):
        self._saved_page_states.append(dict(self.__dict__))
        self._startPage()

    def save(self):
        num_pages = len(self._saved_page_states)
        for state in self._saved_page_states:
            self.__dict__.update(state)
            self.draw_page_decorations(num_pages)
            super().showPage()
        super().save()

    def draw_page_decorations(self, page_count):
        self.saveState()
        self.setFont("Helvetica-Bold", 7)
        self.setFillColor(colors.HexColor("#64748b"))

        # Header (Pages > 1)
        if self._pageNumber > 1:
            self.drawString(20 * mm, 285 * mm, "HYDRASTREAM // RELATÓRIO DE AUDITORIA E REMEDIAÇÃO DE SEGURANÇA (DATA PLANE)")
            self.drawRightString(190 * mm, 285 * mm, "STATUS: 100% REMEDIADO")
            self.setStrokeColor(colors.HexColor("#e2e8f0"))
            self.setLineWidth(0.5)
            self.line(20 * mm, 282 * mm, 190 * mm, 282 * mm)

        # Footer (All pages)
        self.setStrokeColor(colors.HexColor("#e2e8f0"))
        self.setLineWidth(0.5)
        self.line(20 * mm, 15 * mm, 190 * mm, 15 * mm)
        self.drawString(20 * mm, 10 * mm, "Ecossistema Hydra — HydraStream Data Plane v1.0.0 • Auditoria de 5 Categorias")
        self.drawRightString(190 * mm, 10 * mm, f"Página {self._pageNumber} de {page_count}")
        self.restoreState()

def generate_charts(output_dir):
    os.makedirs(output_dir, exist_ok=True)

    # 1. Donut Chart - Status Remediation
    labels = ['Remediadas (100%)', 'Pendentes (0%)']
    sizes = [11, 0]
    chart_colors = ['#10b981', '#cbd5e1']

    fig, ax = plt.subplots(figsize=(3.8, 3.0), dpi=200)
    wedges, texts, autotexts = ax.pie(
        sizes, labels=labels, autopct='%1.0f%%', startangle=90,
        colors=chart_colors, pctdistance=0.75,
        textprops=dict(color='#1e293b', size=9, weight='bold')
    )
    for at in autotexts:
        at.set_color('white')
        at.set_weight('bold')
    centre_circle = plt.Circle((0, 0), 0.52, fc='white')
    fig.gca().add_artist(centre_circle)
    ax.set_title("Status de Remediação (11/11 Concluídas)", fontsize=10, weight='bold', pad=10, color='#0f172a')
    plt.tight_layout()
    chart1_path = os.path.join(output_dir, "chart_severity.png")
    plt.savefig(chart1_path, transparent=True)
    plt.close()

    # 2. Bar Chart - Remediated Categories
    categories = [
        '1. Isolamento Tenant',
        '2. Permissão Servidor',
        '3. IDOR / Recursos',
        '4. Chaves / Comunicação',
        '5. SSRF & Inputs'
    ]
    counts = [2, 2, 2, 2, 3] # Total 11 remediadas
    bar_colors = ['#10b981', '#10b981', '#10b981', '#10b981', '#10b981']

    fig, ax = plt.subplots(figsize=(5.2, 3.0), dpi=200)
    bars = ax.barh(categories[::-1], counts[::-1], color=bar_colors[::-1], height=0.55)
    ax.set_xlim(0, 4)
    ax.set_xlabel("Vulnerabilidades Corrigidas", fontsize=8, weight='bold', color='#475569')
    ax.set_title("Falhas Remediadas por Categoria", fontsize=10, weight='bold', pad=10, color='#0f172a')
    ax.spines['top'].set_visible(False)
    ax.spines['right'].set_visible(False)
    ax.spines['left'].set_color('#cbd5e1')
    ax.spines['bottom'].set_color('#cbd5e1')
    ax.tick_params(axis='both', labelsize=8)
    for bar in bars:
        width = bar.get_width()
        ax.text(width + 0.08, bar.get_y() + bar.get_height()/2, f'{int(width)} [OK]',
                va='center', ha='left', fontsize=8, weight='bold', color='#065f46')
    plt.tight_layout()
    chart2_path = os.path.join(output_dir, "chart_categories.png")
    plt.savefig(chart2_path, transparent=True)
    plt.close()

    return chart1_path, chart2_path

def build_pdf(filename="relatorio-auditoria-seguranca.pdf"):
    output_dir = os.path.dirname(os.path.abspath(filename))
    chart1_path, chart2_path = generate_charts(output_dir)

    doc = SimpleDocTemplate(
        filename,
        pagesize=A4,
        leftMargin=18 * mm,
        rightMargin=18 * mm,
        topMargin=22 * mm,
        bottomMargin=20 * mm,
    )

    styles = getSampleStyleSheet()

    title_style = ParagraphStyle(
        'CoverTitle',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=18,
        leading=22,
        textColor=colors.HexColor('#0f172a'),
        spaceAfter=4,
    )

    subtitle_style = ParagraphStyle(
        'CoverSubtitle',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=11,
        leading=15,
        textColor=colors.HexColor('#ff5e3a'),
        spaceAfter=10,
    )

    h1_style = ParagraphStyle(
        'Heading1_Custom',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=12,
        leading=16,
        textColor=colors.HexColor('#0f172a'),
        spaceBefore=10,
        spaceAfter=5,
    )

    h2_style = ParagraphStyle(
        'Heading2_Custom',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=9.5,
        leading=13,
        textColor=colors.HexColor('#1e293b'),
        spaceBefore=6,
        spaceAfter=3,
    )

    body_style = ParagraphStyle(
        'Body_Custom',
        parent=styles['Normal'],
        fontName='Helvetica',
        fontSize=8,
        leading=11.5,
        textColor=colors.HexColor('#334155'),
        spaceAfter=5,
    )

    code_style = ParagraphStyle(
        'Code_Custom',
        parent=styles['Normal'],
        fontName='Courier',
        fontSize=7,
        leading=9.5,
        textColor=colors.HexColor('#0f172a'),
        backColor=colors.HexColor('#f1f5f9'),
        borderPadding=3,
        spaceBefore=2,
        spaceAfter=3,
    )

    story = []

    # Header Banner
    story.append(Paragraph("RELATÓRIO TÉCNICO DE AUDITORIA & REMEDIAÇÃO DE SEGURANÇA", title_style))
    story.append(Paragraph("HYDRASTREAM DATA PLANE // INGESTÃO ZERO-COPY & MOTOR DE MÍDIA", subtitle_style))
    story.append(HRFlowable(width="100%", thickness=1.5, color=colors.HexColor('#10b981'), spaceBefore=0, spaceAfter=8))

    # Executive Metadata Box
    meta_data = [
        [
            Paragraph("<b>Projeto:</b> HydraStream Data Plane", body_style),
            Paragraph("<b>Versão / Stack:</b> Go 1.25+ / Rust / MediaMTX / SHM", body_style),
        ],
        [
            Paragraph("<b>Data da Avaliação:</b> 14/09/2026", body_style),
            Paragraph("<b>Status Final:</b> 100% REMEDIADO (11 de 11 Corrigidas)", body_style),
        ],
        [
            Paragraph("<b>Escopo:</b> API REST, Ingestão RTSP, ONVIF Probing, Memória", body_style),
            Paragraph("<b>Conformidade de Testes:</b> 100% Aprovados (Go & Rust)", body_style),
        ]
    ]
    meta_table = Table(meta_data, colWidths=[85*mm, 85*mm])
    meta_table.setStyle(TableStyle([
        ('BACKGROUND', (0, 0), (-1, -1), colors.HexColor('#f0fdf4')),
        ('BOX', (0, 0), (-1, -1), 0.5, colors.HexColor('#86efac')),
        ('VALIGN', (0, 0), (-1, -1), 'MIDDLE'),
        ('PADDING', (0, 0), (-1, -1), 4),
    ]))
    story.append(meta_table)
    story.append(Spacer(1, 6))

    # Executive Summary
    story.append(Paragraph("1. Sumário Executivo & Status de Remediação", h1_style))
    story.append(Paragraph(
        "A auditoria de segurança cibernética e a subsequente remediação no <b>HydraStream</b> foram concluídas com sucesso. "
        "Todas as <b>11 vulnerabilidades</b> mapeadas nas cinco categorias críticas foram neutralizadas através de implementação "
        "de middleware criptográfico de autenticação obrigatória (JWT HMAC-SHA256 e Service API Keys), isolamento estrito de inquilino (multi-tenant), "
        "validação RBAC no servidor com <code>RequireRole</code>, proteção contra SSRF no módulo ONVIF/RTSP probe, e confinamento rígido de arquivos (anti-LFI).",
        body_style
    ))

    # Visual Charts
    chart_table_data = [[Image(chart1_path, width=75*mm, height=55*mm), Image(chart2_path, width=95*mm, height=55*mm)]]
    chart_table = Table(chart_table_data, colWidths=[78*mm, 98*mm])
    chart_table.setStyle(TableStyle([
        ('VALIGN', (0, 0), (-1, -1), 'MIDDLE'),
        ('ALIGN', (0, 0), (-1, -1), 'CENTER'),
        ('PADDING', (0, 0), (-1, -1), 0),
    ]))
    story.append(chart_table)
    story.append(Spacer(1, 6))

    # Vulnerability Table
    story.append(Paragraph("2. Quadro Consolidado de Remediações", h1_style))
    vuln_summary_data = [
        ["ID", "Categoria", "Severidade", "Arquivo Afetado", "Status Final"],
        ["HS-01", "1. Isolamento Tenant", "CRÍTICA", "internal/adapters/primary/http/stream_handler.go", "CORRIGIDO"],
        ["HS-02", "1. Isolamento Tenant", "ALTA", "internal/adapters/secondary/memory/memory_store.go", "CORRIGIDO"],
        ["HS-03", "2. Permissão Servidor", "CRÍTICA", "internal/adapters/primary/http/stream_handler.go", "CORRIGIDO"],
        ["HS-04", "2. Permissão Servidor", "ALTA", "internal/adapters/primary/http/router.go", "CORRIGIDO"],
        ["HS-05", "3. IDOR / Recursos", "CRÍTICA", "internal/adapters/primary/http/stream_handler.go", "CORRIGIDO"],
        ["HS-06", "3. IDOR / Recursos", "ALTA", "internal/adapters/primary/http/stream_handler.go", "CORRIGIDO"],
        ["HS-07", "4. Chaves / Comunicação", "MÉDIA", "cmd/hydrastream/main.go", "CORRIGIDO"],
        ["HS-08", "4. Chaves / Comunicação", "MÉDIA", "internal/adapters/secondary/memory/memory_store.go", "CORRIGIDO"],
        ["HS-09", "5. SSRF / Inputs", "CRÍTICA", "internal/adapters/primary/http/probe_handler.go", "CORRIGIDO"],
        ["HS-10", "5. Path / Inputs", "ALTA", "internal/adapters/primary/http/probe_handler.go", "CORRIGIDO"],
        ["HS-11", "5. Path / Inputs", "ALTA", "internal/adapters/primary/http/stream_handler.go", "CORRIGIDO"]
    ]
    t_vulns = Table(vuln_summary_data, colWidths=[15*mm, 38*mm, 22*mm, 75*mm, 24*mm])
    t_vulns.setStyle(TableStyle([
        ('BACKGROUND', (0, 0), (-1, 0), colors.HexColor('#1e293b')),
        ('TEXTCOLOR', (0, 0), (-1, 0), colors.white),
        ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
        ('FONTSIZE', (0, 0), (-1, -1), 7),
        ('VALIGN', (0, 0), (-1, -1), 'MIDDLE'),
        ('GRID', (0, 0), (-1, -1), 0.5, colors.HexColor('#cbd5e1')),
        ('BACKGROUND', (4, 1), (4, -1), colors.HexColor('#dcfce7')),
        ('TEXTCOLOR', (4, 1), (4, -1), colors.HexColor('#15803d')),
        ('FONTNAME', (4, 1), (4, -1), 'Helvetica-Bold'),
        ('PADDING', (0, 0), (-1, -1), 2.5),
    ]))
    story.append(t_vulns)
    story.append(Spacer(1, 8))

    # Technical Details
    story.append(Paragraph("3. Detalhamento Técnico das Correções Implementadas", h1_style))

    story.append(Paragraph("CATEGORIA 1 & 3: ISOLAMENTO DE TENANT & PREVENÇÃO DE IDOR", h2_style))
    story.append(Paragraph(
        "• <b>Middleware de Contexto:</b> <code>middleware.AuthMiddleware</code> extrai o <code>tenant_id</code> e papel autenticado.<br/>"
        "• <b>Filtro Obrigatório:</b> <code>GET /api/v1/streams</code> agora injeta estritamente o <code>tenant_id</code> do contexto autenticado.<br/>"
        "• <b>Validação de Posse (Anti-IDOR):</b> Endpoints <code>/snapshot</code>, <code>/mjpeg</code>, <code>/stats</code> e <code>DELETE</code> verificam se a câmera pertence à mesma organização antes de entregar o fluxo.",
        body_style
    ))

    story.append(Paragraph("CATEGORIA 2: CONTROLE DE ACESSO RBAC NO SERVIDOR", h2_style))
    story.append(Paragraph(
        "• <b>Guarda RBAC:</b> <code>RequireRole(\"admin\", \"superadmin\")</code> aplicado sobre <code>chaos/inject</code>, <code>chaos/reset</code> e mutações de analíticos.<br/>"
        "• <b>Bloqueio de Não Autenticados:</b> Todas as rotas <code>/api/v1/*</code> rejeitam requisições sem token com HTTP 401 Unauthorized.",
        body_style
    ))

    story.append(Paragraph("CATEGORIA 5: PROTEÇÃO SSRF & HIGIENIZAÇÃO DE ENTRADAS", h2_style))
    story.append(Paragraph(
        "• <b>SSRF Guard (<code>isBlockedIPTarget</code>):</b> Bloqueia requisições contra <code>127.0.0.1</code>, <code>::1</code> e metadados cloud (<code>169.254.169.254</code>).<br/>"
        "• <b>Confinamento LFI:</b> <code>probeLoopFile</code> sanitiza caminhos e restringe o acesso estritamente ao diretório <code>samples/</code> com <code>filepath.Clean</code>.<br/>"
        "• <b>Regex Alfanumérico:</b> Identificadores de streams são validados contra <code>^[a-zA-Z0-9_-]{1,64}$</code> antes de qualquer chamada ao FFmpeg.",
        body_style
    ))

    doc.build(story, canvasmaker=NumberedCanvas)
    print(f"✅ PDF successfully generated at: {filename}")

if __name__ == "__main__":
    pdf_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "relatorio-auditoria-seguranca.pdf")
    build_pdf(pdf_path)
