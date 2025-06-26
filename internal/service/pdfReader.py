import PyPDF2
import sys
import json

class PDFReaders:
    def __init__(self, file_path):
        self.file_path = file_path
        self.file = None
        self.reader = None

    def open_pdf(self):
        """Открываем PDF-файл для чтения."""
        self.file = open(self.file_path, 'rb')
        self.reader = PyPDF2.PdfReader(self.file)
    
    def close_pdf(self):
        """Закрываем PDF-файл."""
        if self.file:
            self.file.close()
    
    def get_number_of_pages(self):
        """Возвращает количество страниц в PDF-файле."""
        return len(self.reader.pages) if self.reader else 0
    
    def extract_text_from_page(self, page_num):
        """Извлекает текст с указанной страницы PDF."""
        if self.reader and page_num < len(self.reader.pages):
            page = self.reader.pages[page_num]
            return page.extract_text()
        return ""

    def determine_language(self):
        """Определяет язык PDF по ключевым словам на первой странице."""
        if self.reader:
            first_page_text = self.extract_text_from_page(0)
            if "Счет на оплату" in first_page_text or "Фискальный чек" in first_page_text:
                return 'russian'
            elif "Төлем шоты" in first_page_text or "Фискалдық түбіртек" in first_page_text:
                return 'kazakh'
            elif "Сатып алғаным" in first_page_text:
                return 'kazakh'
            elif "Покупки" in first_page_text:
                return 'russian'
        return 'unknown'
    
    def extract_detailed_info(self):
        """Извлекает каждую строку как отдельный элемент массива, учитывая ключевые слова на казахском и русском языках."""
        language = self.determine_language()
        
        # Определение ключевых слов в зависимости от языка
        if language == 'kazakh':
            specific_keywords = [
                "Фискалдық түбіртек", "ИП", "Төлем сәтті өтті", "₸", "Сату", "Фото и видео",
                "Түбіртек №", "QR", "Күні мен уақыты", "Төленді", "Мекенжай", 
                "Сатушының ЖСН/БСН", "Сатып алушының аты-жөні", "МТН", "МЗН", "ФБ", "ФДО"
            ]
        elif language == 'russian':
            specific_keywords = [
                "Фискальный чек", "ИП", "Платеж успешно совершен", "₸", "Продажа", "Фото и видео",
                "№ чека", "QR", "Дата и время", "Оплачено", "Адрес", 
                "ИИН/БИН продавца", "ФИО покупателя", "РНМ", "ЗНМ", "ФП", "ОФД"
            ]
        else:
            return ["Language not recognized."]

        # Извлечение строк, содержащих ключевые слова
        result_lines = []
        number_of_pages = self.get_number_of_pages()
        for page_num in range(number_of_pages):
            text = self.extract_text_from_page(page_num)
            if text:  # Проверяем, что текст не пустой
                lines = text.split('\n')
                for line in lines:
                    clean_line = line.strip()
                    if clean_line and any(keyword in clean_line for keyword in specific_keywords):
                        result_lines.append(clean_line)
                        
        return result_lines

def main():
    # Check if file path is provided as command line argument
    if len(sys.argv) != 2:
        print(json.dumps(["Error: No file path provided"]), flush=True)
        sys.exit(1)
    
    file_path = sys.argv[1]
    
    try:
        # Initialize PDFReader with the provided file path
        pdf_reader = PDFReaders(file_path)
        
        # Open PDF for reading
        pdf_reader.open_pdf()
        
        # Extract detailed information
        detailed_info = pdf_reader.extract_detailed_info()
        
        # Output as JSON for easier parsing in Go
        print(json.dumps(detailed_info, ensure_ascii=False), flush=True)
        
        # Close PDF after reading
        pdf_reader.close_pdf()
        
    except FileNotFoundError:
        print(json.dumps([f"Error: File not found: {file_path}"]), flush=True)
        sys.exit(1)
    except Exception as e:
        print(json.dumps([f"Error: {str(e)}"]), flush=True)
        sys.exit(1)

if __name__ == "__main__":
    main()