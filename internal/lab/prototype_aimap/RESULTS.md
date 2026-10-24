# Results, 2026-10-24 (openai/gpt-6-luna via OpenRouter, provider.zdr, served by Azure)

```
RIGHT  9. PUSVMC Daily      Sales     sheet='01.09.26' +29 header=8 unpivot=False map={'date': 'Date', 'amount': 'Total Amount', 'staff': 'Created By'}  wrong=[]
wrong  High movement        HR        sheet='all item available' +6 header=1 unpivot=False map={'name': 'Product Name', 'department': 'Sheet'}  wrong=['template', 'sheets']
RIGHT  Service & Product    Sales     sheet='June (Service)' +5 header=1 unpivot=False map={'date': 'Created Date & Time', 'amount': 'Price', 'item': 'Product / Service', 'category': 'Type', 'staff': 'Created By'}  wrong=[]
RIGHT  Stock_Balance        Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Actual Stock', 'category': 'Category', 'price': 'Selling Price'}  wrong=[]
RIGHT  Stock_Detail         General   sheet='Sheet1' +0 header=1 unpivot=False map={}  wrong=[]
wrong  Stock_Movement       Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Purchase Quantity'}  wrong=['mapping']
wrong  Test & Parameters    General   sheet='Test & Parameters' +0 header=1 unpivot=False map={}  wrong=['sheets']
RIGHT  roster               Roster    sheet='Veterinary' +7 header=6 unpivot=True(0, 0) map={'staff': 'Staff', 'date': 'Date', 'code': 'Code', 'department': 'Sheet'}  wrong=[]
baseline: 5/8 right

== AI, raw
RIGHT  9. PUSVMC Daily      Sales     sheet='01.09.26' +29 header=8(2) unpivot=False map={'date': 'Date', 'amount': 'Total Amount', 'staff': 'Created By'}  wrong=[]
       Azure prompt=3778 completion=953 cost=$0.000948675
wrong  High movement        Inventory sheet='all item available' +0 header=1 unpivot=False map={'item': 'Product Name', 'category': 'category', 'price': 'selling price'}  wrong=['template']
       Azure prompt=3167 completion=302 cost=$0.0005468
RIGHT  Service & Product    Sales     sheet='June (Service)' +5 header=1 unpivot=False map={'date': 'Created Date & Time', 'amount': 'Price', 'item': 'Product / Service', 'category': 'Type', 'staff': 'Created By'}  wrong=[]
       Azure prompt=3846 completion=337 cost=$0.000649175
RIGHT  Stock_Balance        Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Actual Stock', 'category': 'Category', 'price': 'Cost Price'}  wrong=[]
       Azure prompt=1381 completion=269 cost=$0.00030705
wrong  Stock_Detail         Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Batch No.', 'quantity': 'Balance'}  wrong=['template']
       Azure prompt=1525 completion=531 cost=$0.00045605
RIGHT  Stock_Movement       Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Closing Balance'}  wrong=[]
       Azure prompt=1356 completion=257 cost=$0.000297925
RIGHT  Test & Parameters    General   sheet='Sheet9' +0 header=1 unpivot=False map={}  wrong=[]
       Azure prompt=2498 completion=304 cost=$0.000464175
RIGHT  roster               Roster    sheet='Veterinary' +7 header=7(2) unpivot=True(10, 2026) map={'staff': 'Staff', 'date': 'Date', 'code': 'Code', 'department': 'Sheet'}  wrong=[]
       Azure prompt=3973 completion=623 cost=$0.00080805
run 1 (raw): 6/8 right, $0.0045 for 8 uploads, 2690 prompt tokens per upload
wrong  9. PUSVMC Daily      Finance   sheet='Cash Closing & Collection - Aug' +0 header=4 unpivot=False map={'date': 'Date', 'amount': 'Total Amount', 'description': 'Reason'}  wrong=['template', 'sheets', 'header']
       Azure prompt=3778 completion=537 cost=$0.00030655
wrong  High movement        Inventory sheet='all item available' +0 header=1 unpivot=False map={'item': 'Product Name', 'category': 'category', 'price': 'selling price'}  wrong=['template']
       Azure prompt=3167 completion=489 cost=$0.00027644
RIGHT  Service & Product    Sales     sheet='June (Service)' +5 header=1 unpivot=False map={'date': 'Created Date & Time', 'amount': 'Price', 'item': 'Product / Service', 'category': 'Type', 'staff': 'Created By'}  wrong=[]
       Azure prompt=3846 completion=303 cost=$0.00019023
RIGHT  Stock_Balance        Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Actual Stock', 'category': 'Category', 'price': 'Cost Price'}  wrong=[]
       Azure prompt=1381 completion=213 cost=$0.00012058
wrong  Stock_Detail         Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Batch No.', 'quantity': 'Balance'}  wrong=['template']
       Azure prompt=1525 completion=389 cost=$0.00021002
RIGHT  Stock_Movement       Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Closing Balance'}  wrong=[]
       Azure prompt=1356 completion=232 cost=$0.00012983
RIGHT  Test & Parameters    General   sheet='Sheet9' +0 header=1 unpivot=False map={}  wrong=[]
       Azure prompt=2498 completion=226 cost=$0.00013825
wrong  roster               Roster    sheet='Veterinary' +0 header=7(2) unpivot=True(10, 2026) map={'staff': 'Staff', 'date': 'Date', 'code': 'Code'}  wrong=['sheets']
       Azure prompt=3973 completion=1092 cost=$0.000586
run 2 (raw): 4/8 right, $0.0020 for 8 uploads, 2690 prompt tokens per upload
RIGHT  9. PUSVMC Daily      Sales     sheet='01.09.26' +29 header=8(2) unpivot=False map={'date': 'Date', 'amount': 'Total Amount', 'staff': 'Created By'}  wrong=[]
       Azure prompt=3778 completion=1044 cost=$0.00056005
wrong  High movement        Inventory sheet='all item available' +0 header=1 unpivot=False map={'item': 'Product Name', 'category': 'category', 'price': 'selling price'}  wrong=['template']
       Azure prompt=3167 completion=567 cost=$0.00031544
RIGHT  Service & Product    Sales     sheet='June (Service)' +5 header=1 unpivot=False map={'date': 'Created Date & Time', 'amount': 'Price', 'item': 'Product / Service', 'category': 'Type', 'staff': 'Created By'}  wrong=[]
       Azure prompt=3846 completion=418 cost=$0.00024773
RIGHT  Stock_Balance        Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Actual Stock', 'category': 'Category', 'price': 'Selling Price'}  wrong=[]
       Azure prompt=1381 completion=270 cost=$0.00014908
RIGHT  Stock_Detail         General   sheet='Sheet1' +0 header=1 unpivot=False map={}  wrong=[]
       Azure prompt=1525 completion=160 cost=$9.552e-05
RIGHT  Stock_Movement       Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Closing Balance'}  wrong=[]
       Azure prompt=1356 completion=267 cost=$0.00014733
RIGHT  Test & Parameters    General   sheet='Sheet5' +0 header=1 unpivot=False map={}  wrong=[]
       Azure prompt=2498 completion=199 cost=$0.00012475
RIGHT  roster               Roster    sheet='Veterinary' +7 header=7(2) unpivot=True(10, 2026) map={'staff': 'Staff', 'date': 'Date', 'code': 'Code'}  wrong=[]
       Azure prompt=3973 completion=1048 cost=$0.000564
run 3 (raw): 7/8 right, $0.0022 for 8 uploads, 2690 prompt tokens per upload

== AI, masked
RIGHT  9. PUSVMC Daily      Sales     sheet='01.09.26' +29 header=8(2) unpivot=False map={'date': 'Date', 'amount': 'Total Sales', 'staff': 'Created By'}  wrong=[]
       Azure prompt=3959 completion=1396 cost=$0.0011928
wrong  High movement        Inventory sheet='all item available' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'available', 'category': 'category', 'price': 'selling price'}  wrong=['template']
       Azure prompt=3416 completion=252 cost=$0.000552925
RIGHT  Service & Product    Sales     sheet='June (Service)' +5 header=1 unpivot=False map={'date': 'Created Date & Time', 'amount': 'Price', 'item': 'Product / Service', 'category': 'Type', 'staff': 'Created By'}  wrong=[]
       Azure prompt=4230 completion=311 cost=$0.000684175
RIGHT  Stock_Balance        Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Actual Stock', 'category': 'Category', 'price': 'Selling Price'}  wrong=[]
       Azure prompt=1452 completion=285 cost=$0.000323925
wrong  Stock_Detail         Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'quantity': 'Balance', 'category': 'Type'}  wrong=['template']
       Azure prompt=1643 completion=474 cost=$0.0004423
RIGHT  Stock_Movement       Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Closing Balance'}  wrong=[]
       Azure prompt=1359 completion=251 cost=$0.0002953
RIGHT  Test & Parameters    General   sheet='Sheet9' +0 header=1 unpivot=False map={}  wrong=[]
       Azure prompt=2881 completion=234 cost=$0.00047705
RIGHT  roster               Roster    sheet='Veterinary' +7 header=7(2) unpivot=True(0, 0) map={'staff': 'Staff', 'date': 'Date', 'code': 'Code', 'department': 'Sheet'}  wrong=[]
       Azure prompt=4135 completion=1395 cost=$0.0012143
run 1 (masked): 6/8 right, $0.0052 for 8 uploads, 2884 prompt tokens per upload
RIGHT  9. PUSVMC Daily      Sales     sheet='01.09.26' +29 header=8(2) unpivot=False map={'date': 'Date', 'amount': 'Total Sales', 'staff': 'Doctor-In- Charge'}  wrong=[]
       Azure prompt=3959 completion=803 cost=$0.00044136
wrong  High movement        Inventory sheet='all item available' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'available', 'category': 'category', 'price': 'selling price'}  wrong=['template']
       Azure prompt=3416 completion=267 cost=$0.00016793
RIGHT  Service & Product    Sales     sheet='June (Service)' +5 header=1 unpivot=False map={'date': 'Created Date & Time', 'amount': 'Price', 'item': 'Product / Service', 'category': 'Type', 'staff': 'Created By'}  wrong=[]
       Azure prompt=4230 completion=320 cost=$0.00020257
RIGHT  Stock_Balance        Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Actual Stock', 'category': 'Category', 'price': 'Cost Price'}  wrong=[]
       Azure prompt=1452 completion=236 cost=$0.00013279
wrong  Stock_Detail         Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Batch No.', 'quantity': 'Balance', 'category': 'Type'}  wrong=['template']
       Azure prompt=1643 completion=308 cost=$0.0001707
RIGHT  Stock_Movement       Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Closing Balance'}  wrong=[]
       Azure prompt=1359 completion=276 cost=$0.00015186
RIGHT  Test & Parameters    General   sheet='Sheet5' +0 header=1 unpivot=False map={}  wrong=[]
       Azure prompt=2881 completion=464 cost=$0.00026108
wrong  roster               Roster    sheet='Surgery Dept' +1 header=5(2) unpivot=True(0, 0) map={'staff': 'Staff', 'date': 'Date', 'code': 'Code'}  wrong=['sheets', 'header']
       Azure prompt=4135 completion=1828 cost=$0.00095562
run 2 (masked): 5/8 right, $0.0025 for 8 uploads, 2884 prompt tokens per upload
RIGHT  9. PUSVMC Daily      Sales     sheet='01.09.26' +29 header=8(2) unpivot=False map={'date': 'Date', 'amount': 'Total Sales', 'staff': 'Doctor-In- Charge'}  wrong=[]
       Azure prompt=3959 completion=1257 cost=$0.00066836
wrong  High movement        Inventory sheet='all item available' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'available', 'category': 'category', 'price': 'selling price'}  wrong=['template']
       Azure prompt=3416 completion=419 cost=$0.00024393
RIGHT  Service & Product    Sales     sheet='June (Service)' +5 header=1 unpivot=False map={'date': 'Created Date & Time', 'amount': 'Price', 'item': 'Product / Service', 'category': 'Type', 'staff': 'Created By'}  wrong=[]
       Azure prompt=4230 completion=453 cost=$0.00026907
RIGHT  Stock_Balance        Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Actual Stock', 'category': 'Category', 'price': 'Cost Price'}  wrong=[]
       Azure prompt=1452 completion=229 cost=$0.00012929
RIGHT  Stock_Detail         General   sheet='Sheet1' +0 header=1 unpivot=False map={}  wrong=[]
       Azure prompt=1643 completion=122 cost=$7.77e-05
RIGHT  Stock_Movement       Inventory sheet='Sheet1' +0 header=1 unpivot=False map={'item': 'Product Name', 'quantity': 'Closing Balance'}  wrong=[]
       Azure prompt=1359 completion=195 cost=$0.00011136
RIGHT  Test & Parameters    General   sheet='Sheet9' +0 header=1 unpivot=False map={}  wrong=[]
       Azure prompt=2881 completion=329 cost=$0.00019358
RIGHT  roster               Roster    sheet='Veterinary' +7 header=7(2) unpivot=True(8, 2024) map={'staff': 'B', 'date': 'Date', 'code': 'Code'}  wrong=[]
       Azure prompt=4135 completion=1340 cost=$0.00071162
run 3 (masked): 7/8 right, $0.0024 for 8 uploads, 2884 prompt tokens per upload
```
