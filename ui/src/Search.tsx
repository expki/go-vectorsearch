import { useEffect, useState } from 'react';
import { Form, Card, Table, Button, Row, Col, Spinner } from 'react-bootstrap';

import { Search as ApiSearch } from './api/search';
import { DeleteDocument } from './api/delete';
import { Chat } from './api/chat';

type Props = {
  owner: string,
  category: string,
}

function Search({ owner, category }: Props) {
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [searchResults, setSearchResults] = useState<Array<result>|undefined>(undefined);
  const [searching, setSearching] = useState<boolean>(false);
  const [currentPage, setCurrentPage] = useState<number>(1);
  const itemsPerPage = 3;

  useEffect(() => {
    setSearchResults(undefined);
  }, [category]);

  const handlePressSearch = () => {
    setCurrentPage(1);
    handleSearch(itemsPerPage, 0);
  }

  const handleSearch = (count: number, offset: number) => {
    setSearching(true);
    ApiSearch({
      owner: owner,
      category: category,
      prefix: category,
      text: searchQuery.trim(),
      count: count,
      offset: offset,
    }).then((res) => {
      const documents = res?.documents ?? [];
      const results: Array<result> = documents.map((document, idx) => ({
        id: document.document_id,
        title: `Result ${idx+1} has ${(100 * document.document_similarity).toFixed(2)}% similarity`,
        description: String(document.document),
      }));
      setSearchResults(results);
      setSearching(false);
    });
  };

  const handleNext = () => {
    const nextPage = currentPage + 1;
    setCurrentPage(nextPage);
    handleSearch(itemsPerPage, (nextPage - 1) * itemsPerPage);
  }

  const handlePrevious = () => {
    const previousPage = currentPage - 1;
    if (previousPage <= 0) return;
    setCurrentPage(previousPage);
    handleSearch(itemsPerPage, (previousPage - 1) * itemsPerPage);
  }

  const handleDeleteDocument = (documentID: number) => {
    DeleteDocument(owner, category, documentID);
    const updatedSearchResults = [...(searchResults ?? [])].filter((item) => item.id !== documentID);
    setSearchResults(updatedSearchResults);
  }

  return (
    <>
      <Card bg="dark" text="light" className="mb-4 rounded-3 border-secondary">
        <Card.Body>
          <h2 className="mb-4 text-center">Vector Search</h2>
          <Form>
            <Form.Group className="mb-3">
              <Form.Control
                type="text"
                placeholder="Enter your search query..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key !== 'Enter') {
                    return;
                  }
                  e.preventDefault();
                  handlePressSearch();
                }}
                className="form-control-lg bg-dark text-light rounded-3 border-secondary"
              />
              <div className="text-muted mt-2 small">
                Searching in category: <span className="fw-bold">{category}</span>
              </div>
              <Button 
                variant="primary" 
                className="w-100 rounded-3 mt-2"
                onClick={handlePressSearch}
                disabled={searching}
              >
                {searching ?
                  <Spinner animation="border" role="status">
                    <span className="visually-hidden">Searching...</span>
                  </Spinner>
                  : 
                  <>Search</>
                }
              </Button>
            </Form.Group>
          </Form>
        </Card.Body>
      </Card>
      
      {/* Search Results */}
      {searchResults !== undefined && (
        <Card bg="dark" text="light" className="rounded-3 border-secondary mb-4">
          <Card.Header className="border-secondary">
            <h4>Search Results</h4>
          </Card.Header>
          <Card.Body>
            <Table variant="dark" className="border-secondary">
              <tbody>
                {(searchResults ?? []).map((result) => (
                  <Result key={result.id} details={result} handleDeleteDocument={handleDeleteDocument} />
                ))}
              </tbody>
            </Table>
            <div className="d-flex justify-content-between align-items-center mt-3">
              <div className="text-muted">
                Showing page {currentPage} results
              </div>
              <div>
                <Button
                  variant="outline-primary"
                  size="sm"
                  className="me-2"
                  onClick={() => handlePrevious()}
                  disabled={currentPage <= 1}
                >
                  Previous
                </Button>
                <Button
                  variant="outline-primary"
                  size="sm"
                  onClick={() => handleNext()}
                  disabled={searchResults.length !== itemsPerPage}
                >
                  Next
                </Button>
              </div>
            </div>
          </Card.Body>
        </Card>
      )}
    </>
  );
}

export default Search;

type result = {
  id: number,
  title: string,
  description: string,
}

function Result({ details, handleDeleteDocument }: { details: result, handleDeleteDocument: (id: number) => void }) {
  const [summaryEnabled, setSummaryEnabled] = useState<boolean>(false);
  const [summary, setSummary] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState<boolean>(false);
  
  const handleSummaryToggle = () => {
    const enabled = summaryEnabled;
    setSummaryEnabled(!summaryEnabled);
    if (!enabled && !summary) {
      setLoading(true);
      Chat(
        {
          text: 'Write a summary paragraph',
          document_ids:[details.id],
        },
        (text: string) => setSummary(text),
        () => setLoading(false),
      );
    }
  }

  return (
    <tr>
      <td>
        <div className="search-result p-2 rounded-3">
          <Row>
            <Col>
              <h5 className="text-primary">{details.title}</h5>
            </Col>
            <Col xs="auto">
              <Button 
                variant={summaryEnabled ? "primary" : "outline-primary"}
                size="sm" 
                className="space-right"
                onClick={() => handleSummaryToggle()}
                disabled={loading}
              >
                AI
              </Button>
              <Button 
                variant="outline-danger" 
                size="sm" 
                className="rounded-circle"
                onClick={() => handleDeleteDocument(details.id)}
              >
                X
              </Button>
            </Col>
          </Row>
          <p className="mb-0">{summaryEnabled ? summary : details.description}</p>
        </div>
      </td>
    </tr>
  );
}
