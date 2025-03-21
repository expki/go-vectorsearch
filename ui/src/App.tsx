import { useState, useEffect } from 'react';
import { Container, Row, Col } from 'react-bootstrap';
import Categories from './Categories';
import Content from './Content';
import Search from './Search';
import './App.css';

import { GetCategories } from './api/categories';

function App() {
  // get owner
  const existingOwner = localStorage.getItem('owner');
  const owner = !existingOwner ?
    (() => {
      const newOwner = crypto.randomUUID();
      localStorage.setItem('owner', newOwner);
      return newOwner;
    })() :
    existingOwner;
  
  const [categories, setCategories] = useState<Array<string>>(['']);
  const [category, setCategory] = useState<string>(categories[0]);
  
  useEffect(() => {
    GetCategories({owner: owner}).then((res) => {
      const existingCategories = res?.category_names ?? [];
      if (existingCategories.length === 0) {
        existingCategories.push('General');
      }
      setCategories(existingCategories);
      setCategory(existingCategories[0]);
    });
  }, []);
  

  return (
    <Container fluid className="app-container bg-dark text-light min-vh-100 p-0">
      <Row className="m-0">
        {/* Left Navigation Panel */}
        <Col md={3} className="sidebar p-4">
          <Categories
            owner={owner}
            selectedCategory={category}
            categories={categories}
            setSelectedCategory={setCategory}
            setCategories={setCategories}
          />
        </Col>
        
        {/* Main Content Area */}
        <Col md={9} className="main-content p-4">
          <Search
            owner={owner}
            category={category}
          />
          
          {/* File Upload Section */}
          <Content
            owner={owner}
            category={category}
          />
        </Col>
      </Row>
    </Container>
  );
}

export default App;
