import logging
from dataclasses import dataclass
from datetime import date

import arxiv
from engine.arxiv_id import normalize_arxiv_id

logger = logging.getLogger(__name__)


@dataclass
class PaperMetadata:
    arxiv_id: str
    title: str
    authors: list[str]
    abstract: str
    categories: list[str]
    published: date
    pdf_url: str


def fetch_metadata(arxiv_ids: list[str]) -> list[PaperMetadata]:
    client = arxiv.Client()
    search = arxiv.Search(id_list=arxiv_ids)

    papers = []
    for result in client.results(search):
        arxiv_id = normalize_arxiv_id(result.entry_id.split("/")[-1])
        papers.append(
            PaperMetadata(
                arxiv_id=arxiv_id,
                title=result.title,
                authors=[a.name for a in result.authors],
                abstract=result.summary,
                categories=result.categories,
                published=result.published.date(),
                pdf_url=result.pdf_url,
            )
        )
        logger.info(f"Fetched metadata: {arxiv_id} - {result.title[:50]}")

    return papers
